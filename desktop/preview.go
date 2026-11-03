package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Preview struct {
	Song    Song    `json:"song"`
	URL     string  `json:"url"`
	Quality Quality `json:"quality"`
	Decoded bool    `json:"decoded"`
	Notice  string  `json:"notice"`
}
type previewSession struct {
	Song    Song
	Address string
	Headers map[string]string
	Expires time.Time
	Decode  bool
	Quality Quality
}

func (s *Studio) PreparePreview(id string, decode bool) (Preview, error) {
	s.mu.Lock()
	song, ok := s.known[id]
	s.mu.Unlock()
	if !ok {
		return Preview{}, errors.New("أعد البحث عن الأغنية")
	}
	if song.Provider == "spotify" {
		return Preview{}, errors.New("Spotify كتالوج بيانات؛ اختر نتيجة مطابقة من مصدر صوت لتشغيلها")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 65*time.Second)
	defer cancel()
	address := song.AudioURL
	headers := map[string]string{}
	q := Quality{Codec: song.Codec, Bitrate: song.Bitrate, Rate: song.SampleRate, Bits: song.Bits, Duration: song.Duration}
	if extractorSource(song.Provider) && address == "" {
		var info struct {
			URL        string  `json:"url"`
			Title      string  `json:"title"`
			Track      string  `json:"track"`
			Artist     string  `json:"artist"`
			Uploader   string  `json:"uploader"`
			Album      string  `json:"album"`
			Duration   float64 `json:"duration"`
			Thumbnail  string  `json:"thumbnail"`
			Thumbnails []struct {
				URL string `json:"url"`
			} `json:"thumbnails"`
			Headers  map[string]string `json:"http_headers"`
			Protocol string            `json:"protocol"`
			Codec    string            `json:"acodec"`
			ABR      float64           `json:"abr"`
			Rate     float64           `json:"asr"`
			DRM      bool              `json:"has_drm"`
		}
		args := []string{"--ignore-config", "--no-plugin-dirs", "--no-cache-dir", "--no-playlist", "--socket-timeout", "18", "--retries", "1", "--no-warnings", "--dump-single-json", "-f", "bestaudio[protocol=https]/bestaudio[protocol=http]/bestaudio/best", "--", song.URL}
		cmd := s.command(ctx, "yt-dlp", args...)
		var out, errText limitedBuffer
		out.Limit = 16 << 20
		errText.Limit = 4000
		cmd.Stdout = &out
		cmd.Stderr = &errText
		if e := cmd.Run(); e != nil {
			return Preview{}, errors.New(trimError(errText.Bytes()))
		}
		if e := json.Unmarshal(out.Bytes(), &info); e != nil {
			return Preview{}, e
		}
		if info.DRM {
			return Preview{}, errors.New("المصدر يعرض ملفًا محميًا")
		}
		address = info.URL
		headers = info.Headers
		if info.Track != "" {
			song.Title = info.Track
		}
		if info.Artist != "" {
			song.Artist = info.Artist
		} else if song.Artist == "" {
			song.Artist = info.Uploader
		}
		if info.Album != "" {
			song.Album = info.Album
		}
		if info.Duration > 0 {
			song.Duration = info.Duration
		}
		if info.Thumbnail != "" {
			song.Artwork = info.Thumbnail
		} else if len(info.Thumbnails) > 0 {
			song.Artwork = info.Thumbnails[len(info.Thumbnails)-1].URL
		}
		decode = decode || strings.Contains(info.Protocol, "m3u8") || strings.Contains(info.Protocol, "dash")
		q = Quality{Codec: strings.ToUpper(info.Codec), Rate: int(info.Rate), Bitrate: int(info.ABR * 1000), Duration: song.Duration}
		song.Codec = q.Codec
		song.Bitrate = q.Bitrate
		song.SampleRate = q.Rate
	}
	if song.Provider == "tidal" {
		var e error
		address, e = s.tidalAudio(ctx, song)
		if e != nil {
			return Preview{}, e
		}
	}
	if strings.HasPrefix(address, "dash:") {
		decode = true
	} else if validWebURL(address) != nil {
		return Preview{}, errors.New("لا يوجد بث صوت متاح لهذا المسار")
	}
	tokenBytes := make([]byte, 24)
	if _, e := rand.Read(tokenBytes); e != nil {
		return Preview{}, e
	}
	token := hex.EncodeToString(tokenBytes)
	s.previewMu.Lock()
	if s.previews == nil {
		s.previews = map[string]previewSession{}
	}
	for id, v := range s.previews {
		if time.Now().After(v.Expires) {
			delete(s.previews, id)
		}
	}
	if len(s.previews) > 64 {
		for id := range s.previews {
			delete(s.previews, id)
			break
		}
	}
	s.previews[token] = previewSession{Song: song, Address: address, Headers: headers, Expires: time.Now().Add(2 * time.Hour), Decode: decode, Quality: q}
	s.previewMu.Unlock()
	s.mu.Lock()
	s.known[id] = song
	if s.previewSongs == nil {
		s.previewSongs = map[string]Song{}
	}
	s.previewSongs[id] = song
	s.mu.Unlock()
	notice := "بث مباشر قبل التنزيل · جودة معلنة من المصدر"
	if decode {
		notice = "بث مع فك محلي للتوافق؛ الملف الأصلي يبقى لدى المصدر"
	}
	s.previewMu.Lock()
	base := s.previewAddress
	serverError := s.previewServerError
	s.previewMu.Unlock()
	if serverError != "" {
		return Preview{}, errors.New("تعذر فتح قناة البث المحلية: " + serverError)
	}
	return Preview{Song: song, URL: base + "/stream/" + token, Quality: q, Decoded: decode, Notice: notice}, nil
}
func previewHeaders(headers map[string]string) http.Header {
	out := http.Header{}
	for key, value := range headers {
		switch strings.ToLower(key) {
		case "user-agent", "referer", "origin", "accept", "accept-language":
			if !strings.ContainsAny(value, "\r\n") {
				out.Set(key, value)
			}
		}
	}
	if out.Get("User-Agent") == "" {
		out.Set("User-Agent", "MadarMusic/1.0")
	}
	return out
}
func (s *Studio) servePreview(w http.ResponseWriter, r *http.Request, token string) {
	if r.Method != "GET" && r.Method != "HEAD" {
		w.WriteHeader(405)
		return
	}
	s.previewMu.Lock()
	session, ok := s.previews[token]
	s.previewMu.Unlock()
	if !ok || time.Now().After(session.Expires) {
		http.Error(w, "البث انتهى؛ اضغط تشغيل مجددًا", 410)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if session.Decode {
		s.serveDecodedPreview(w, r, session)
		return
	}
	req, e := http.NewRequestWithContext(r.Context(), r.Method, session.Address, nil)
	if e != nil {
		http.Error(w, "بث غير صالح", 502)
		return
	}
	req.Header = previewHeaders(session.Headers)
	for _, key := range []string{"Range", "If-Range"} {
		if v := r.Header.Get(key); v != "" {
			req.Header.Set(key, v)
		}
	}
	client := *s.client
	client.Timeout = 0
	res, e := client.Do(req)
	if e != nil {
		http.Error(w, "تعذر الاتصال بمصدر البث", 502)
		return
	}
	defer res.Body.Close()
	if res.StatusCode != 200 && res.StatusCode != 206 {
		http.Error(w, fmt.Sprintf("مصدر البث HTTP %d", res.StatusCode), res.StatusCode)
		return
	}
	kind := strings.ToLower(res.Header.Get("Content-Type"))
	if strings.Contains(kind, "text/html") || strings.Contains(kind, "json") {
		http.Error(w, "المصدر لم يرجع صوتًا", 502)
		return
	}
	for _, key := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified"} {
		if v := res.Header.Get(key); v != "" {
			w.Header().Set(key, v)
		}
	}
	w.WriteHeader(res.StatusCode)
	if r.Method != "HEAD" {
		io.Copy(w, res.Body)
	}
}
func (s *Studio) serveDecodedPreview(w http.ResponseWriter, r *http.Request, session previewSession) {
	if r.Method == "HEAD" {
		w.Header().Set("Content-Type", "audio/wav")
		return
	}
	address := session.Address
	protocols := "http,https,tcp,tls"
	if strings.HasPrefix(address, "dash:") {
		address = strings.TrimPrefix(address, "dash:")
		protocols = "file,http,https,tcp,tls"
	}
	args := []string{"-v", "error", "-nostdin", "-rw_timeout", "15000000", "-protocol_whitelist", protocols}
	if strings.HasPrefix(address, "http") {
		h := previewHeaders(session.Headers)
		var lines strings.Builder
		for _, key := range []string{"User-Agent", "Referer", "Origin"} {
			if value := h.Get(key); value != "" {
				lines.WriteString(key + ": " + value + "\r\n")
			}
		}
		args = append(args, "-headers", lines.String())
	}
	args = append(args, "-i", address, "-map", "0:a:0", "-vn", "-ac", "2", "-c:a", "pcm_f32le", "-f", "wav", "pipe:1")
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	cmd := s.command(ctx, "ffmpeg", args...)
	stdout, e := cmd.StdoutPipe()
	if e != nil {
		http.Error(w, "تعذر تجهيز البث", 502)
		return
	}
	var errText limitedBuffer
	errText.Limit = 3000
	cmd.Stderr = &errText
	if e = cmd.Start(); e != nil {
		http.Error(w, "محرك الصوت غير موجود", 502)
		return
	}
	reader := bufio.NewReader(stdout)
	if _, e = reader.Peek(12); e != nil {
		cancel()
		cmd.Wait()
		http.Error(w, "تعذر فك بث المصدر", 502)
		return
	}
	w.Header().Set("Content-Type", "audio/wav")
	w.Header().Set("Accept-Ranges", "none")
	io.Copy(w, reader)
	cancel()
	cmd.Wait()
}
func (s *Studio) PreviewLyrics(id string, refresh bool) (LyricView, error) {
	return s.GetLyrics(id, refresh)
}
