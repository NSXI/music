package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (s *Studio) searchAll(ctx context.Context, query string) (Result, error) {
	names := append([]string{"soundcloud", "bandcamp", "archive"}, extraSources...)
	if s.GetState().Preferences.TidalURL != "" {
		names = append(names, "tidal")
	}
	if s.GetState().SpotifyConnected {
		names = append(names, "spotify")
	}
	type response struct {
		Name   string
		Result Result
		Error  error
	}
	channel := make(chan response, len(names))
	for _, name := range names {
		go func(provider string) {
			run, cancel := context.WithTimeout(ctx, 80*time.Second)
			defer cancel()
			var r Result
			var e error
			r, e = s.providerSearch(run, provider, query)
			channel <- response{provider, r, e}
		}(name)
	}
	out := Result{Title: query, Tracks: []Song{}, Complete: false, ProviderCounts: map[string]int{}, ProviderErrors: map[string]string{}}
	for range names {
		r := <-channel
		if r.Error != nil {
			out.ProviderErrors[r.Name] = r.Error.Error()
			continue
		}
		out.ProviderCounts[r.Name] = len(r.Result.Tracks)
		out.Tracks = append(out.Tracks, r.Result.Tracks...)
	}
	sort.SliceStable(out.Tracks, func(i, j int) bool {
		a, b := out.Tracks[i], out.Tracks[j]
		if norm(a.Title) == norm(b.Title) {
			return a.Provider < b.Provider
		}
		return norm(a.Title) < norm(b.Title)
	})
	out.Total = len(out.Tracks)
	out.Notice = fmt.Sprintf("بحث في %d مصادر مهيأة؛ %d نتيجة. اختر المصدر من الفلاتر. روابط الملفات المباشرة تُفحص عند لصقها.", len(names), len(out.Tracks))
	if s.GetState().Preferences.TidalURL == "" {
		out.ProviderErrors["tidal"] = "يحتاج خادمًا متوافقًا؛ لم يُستخدم حسابك"
	}
	if len(out.Tracks) == 0 && len(out.ProviderErrors) > 0 {
		return out, errors.New("لم تُرجع المصادر المتاحة نتائج. جرّب رابطًا مباشرًا أو راجع حالة المصادر.")
	}
	return out, nil
}

type formatInfo struct {
	ID       string  `json:"format_id"`
	Ext      string  `json:"ext"`
	Codec    string  `json:"acodec"`
	Video    string  `json:"vcodec"`
	Size     float64 `json:"filesize"`
	Estimate float64 `json:"filesize_approx"`
	ABR      float64 `json:"abr"`
	ASR      float64 `json:"asr"`
	DRM      bool    `json:"has_drm"`
}

func (s *Studio) InspectSong(id string) (Song, error) {
	s.mu.Lock()
	song, ok := s.known[id]
	s.mu.Unlock()
	if !ok {
		return Song{}, errors.New("أعد البحث عن المسار")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	switch song.Provider {
	case "soundcloud", "bandcamp", "ytmusic", "hearthis", "mixcloud", "jamendo", "audiomack", "reverbnation":
		args := []string{"--ignore-config", "--no-plugin-dirs", "--no-cache-dir", "--no-playlist", "--socket-timeout", "15", "--retries", "1", "--dump-single-json", "--no-warnings", "--", song.URL}
		cmd := s.command(ctx, "yt-dlp", args...)
		var out, errText limitedBuffer
		out.Limit = 12 << 20
		errText.Limit = 3000
		cmd.Stdout = &out
		cmd.Stderr = &errText
		if e := cmd.Run(); e != nil {
			return song, errors.New(trimError(errText.Bytes()))
		}
		var data struct {
			Thumbnail  string `json:"thumbnail"`
			Thumbnails []struct {
				URL string `json:"url"`
			} `json:"thumbnails"`
			Formats []formatInfo `json:"formats"`
			Size    float64      `json:"filesize"`
			Ext     string       `json:"ext"`
			Codec   string       `json:"acodec"`
			ABR     float64      `json:"abr"`
		}
		if e := json.Unmarshal(out.Bytes(), &data); e != nil {
			return song, e
		}
		if data.Thumbnail != "" {
			song.Artwork = data.Thumbnail
		} else if len(data.Thumbnails) > 0 {
			song.Artwork = data.Thumbnails[len(data.Thumbnails)-1].URL
		}
		formats := []formatInfo{}
		for _, f := range data.Formats {
			if !f.DRM && f.Codec != "none" {
				formats = append(formats, f)
			}
		}
		sort.SliceStable(formats, func(i, j int) bool {
			rank := func(f formatInfo) float64 {
				if f.ID == "download" || f.Ext == "flac" || f.Ext == "wav" {
					return 1e9 + f.ABR
				}
				return f.ABR
			}
			return rank(formats[i]) > rank(formats[j])
		})
		if len(formats) > 0 {
			f := formats[0]
			song.Codec = strings.ToUpper(f.Codec)
			if song.Codec == "" || song.Codec == "UNKNOWN" {
				song.Codec = strings.ToUpper(f.Ext)
			}
			song.Size = int64(f.Size)
			if song.Size == 0 {
				song.Size = int64(f.Estimate)
				song.SizeEstimated = song.Size > 0
			}
			song.Bitrate = int(f.ABR * 1000)
			song.SampleRate = int(f.ASR)
		} else {
			song.Codec = strings.ToUpper(data.Codec)
			song.Size = int64(data.Size)
			song.Bitrate = int(data.ABR * 1000)
		}
	case "spotify":
		return song, errors.New("Spotify يعرض بيانات التسجيل. اختر نتيجة من مصدر صوت لفحص الملف")
	default:
		address := song.AudioURL
		if song.Provider == "tidal" {
			var e error
			address, e = s.tidalAudio(ctx, song)
			if e != nil {
				return song, e
			}
			if strings.HasPrefix(address, "dash:") {
				song.Codec = "DASH audio"
				break
			}
		}
		if address == "" {
			return song, errors.New("لا يوجد رابط صوت متاح للفحص")
		}
		req, _ := http.NewRequestWithContext(ctx, "HEAD", address, nil)
		res, e := s.client.Do(req)
		if e == nil {
			if res.StatusCode == 200 && res.ContentLength > 0 {
				song.Size = res.ContentLength
			}
			res.Body.Close()
		}
		out, e := s.command(ctx, "ffprobe", "-v", "error", "-rw_timeout", "12000000", "-probesize", "262144", "-analyzeduration", "2000000", "-print_format", "json", "-show_format", "-show_streams", address).Output()
		if e == nil {
			var data probeData
			if json.Unmarshal(out, &data) == nil {
				for _, f := range data.Streams {
					if f.Type != "audio" {
						continue
					}
					song.Codec = strings.ToUpper(f.Codec)
					song.SampleRate, _ = strconv.Atoi(f.Rate)
					song.Bitrate, _ = strconv.Atoi(f.Bitrate)
					song.Bits = f.Bits
					if raw, _ := strconv.Atoi(f.RawBits); raw > 0 {
						song.Bits = raw
					}
					song.QualityVerified = true
					break
				}
			}
		}
		if song.Codec == "" && song.Size <= 0 {
			return song, errors.New("المصدر لم يتيح معلومات الملف قبل التنزيل")
		}
	}
	if song.Size == 0 && song.Bitrate > 0 && song.Duration > 0 {
		song.Size = int64(song.Duration * float64(song.Bitrate) / 8)
		song.SizeEstimated = true
	}
	s.mu.Lock()
	s.known[id] = song
	s.mu.Unlock()
	return song, nil
}
