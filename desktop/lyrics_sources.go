package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var lyricProviderNames = map[string]string{"lrclib": "LRCLIB", "netease": "NetEase Music", "qq": "QQ Music", "kugou": "Kugou Music", "musixmatch": "Musixmatch", "local-align": "مزامنة مدار المحلية", "local": "ملف محلي"}
var defaultLyricProviders = []string{"lrclib", "qq", "netease", "kugou", "musixmatch"}

type TimedWord struct {
	Text        string  `json:"text"`
	Start       float64 `json:"start"`
	End         float64 `json:"end"`
	Probability float64 `json:"probability"`
}
type WordLine struct {
	Start float64     `json:"start"`
	End   float64     `json:"end"`
	Text  string      `json:"text"`
	Words []TimedWord `json:"words"`
}
type LyricCache struct {
	Pronunciation []string `json:"pronunciation,omitempty"`
	Song      Song             `json:"song"`
	Record    LyricsRecord     `json:"record"`
	SavedAt   string           `json:"savedAt"`
	Words     []WordLine       `json:"words,omitempty"`
	Alignment *AlignmentReport `json:"alignment,omitempty"`
}

func lyricKey(song Song) string {
	return key(norm(song.Title) + "|" + norm(song.Artist) + "|" + fmt.Sprintf("%.0f", song.Duration) + "|" + song.ISRC)
}
func validLRC(text string, duration float64) bool {
	matches := regexp.MustCompile(`(?m)\[(\d{1,3}):(\d{2}(?:\.\d+)?)\]\s*([^\r\n]+)`).FindAllStringSubmatch(text, -1)
	count := 0
	for _, m := range matches {
		minutes, _ := strconv.ParseFloat(m[1], 64)
		seconds, _ := strconv.ParseFloat(m[2], 64)
		at := minutes*60 + seconds
		if seconds >= 60 || at < 0 || duration > 0 && at > duration+15 {
			continue
		}
		if norm(m[3]) != "" {
			count++
		}
	}
	return count >= 1
}
func (s *Studio) readLyricCache(song Song) (LyricCache, error) {
	var c LyricCache
	raw, e := os.ReadFile(filepath.Join(s.root, "data", "lyrics", lyricKey(song)+".json"))
	if e != nil {
		return c, e
	}
	e = json.Unmarshal(raw, &c)
	if e != nil {
		return c, e
	}
	if c.Song.ID != song.ID && !sameSong(c.Song, song) {
		return c, errors.New("الكلمات المحفوظة لا تطابق التسجيل")
	}
	return c, nil
}
func (s *Studio) writeLyricCache(c LyricCache) error {
	dir := filepath.Join(s.root, "data", "lyrics")
	if e := os.MkdirAll(dir, 0755); e != nil {
		return e
	}
	c.SavedAt = time.Now().Format(time.RFC3339)
	b, e := json.MarshalIndent(c, "", "  ")
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(dir, ".lyrics-*.tmp")
	if e != nil {
		return e
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, e = tmp.Write(b); e != nil {
		tmp.Close()
		return e
	}
	tmp.Sync()
	tmp.Close()
	return os.Rename(name, filepath.Join(dir, lyricKey(c.Song)+".json"))
}
func (s *Studio) lyricsRequest(ctx context.Context, provider, endpoint string, params url.Values, target any) error {
	base := map[string]string{"lrclib": "https://lrclib.net", "netease": "https://music.163.com", "qq": "https://u.y.qq.com", "kugou": "https://lyrics.kugou.com", "musixmatch": "https://api.musixmatch.com/ws/1.1"}[provider]
	if s.lyricTestBases != nil && s.lyricTestBases[provider] != "" {
		base = s.lyricTestBases[provider]
	}
	address := base + endpoint
	if len(params) > 0 {
		address += "?" + params.Encode()
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", address, nil)
	req.Header.Set("User-Agent", "MadarMusic/1.0")
	if provider == "qq" {
		req.Header.Set("Referer", "https://y.qq.com/")
	}
	res, e := s.client.Do(req)
	if e != nil {
		return e
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", res.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(target)
}
func lyricsRecordingMatch(song Song, title, artist string, duration float64) bool {
	if norm(song.Title) != norm(title) || norm(song.Artist) != norm(artist) {
		return false
	}
	if song.Duration > 0 && duration > 0 {
		d := song.Duration - duration
		if d < 0 {
			d = -d
		}
		return d <= 4
	}
	return true
}
func (s *Studio) fetchLyricProvider(ctx context.Context, song Song, provider string) (LyricsRecord, error) {
	empty := LyricsRecord{}
	switch provider {
	case "amll":
		return s.fetchAMLL(ctx, song)
	case "lrclib":
		params := url.Values{"track_name": {song.Title}, "artist_name": {song.Artist}}
		if song.Duration > 0 {
			params.Set("duration", fmt.Sprintf("%.3f", song.Duration))
		}
		if song.Album != "" {
			params.Set("album_name", song.Album)
		}
		var record LyricsRecord
		e := s.lyricsRequest(ctx, provider, "/api/get", params, &record)
		if e == nil && lyricsRecordingMatch(song, record.Title, record.Artist, record.Duration) && (record.Synced != "" || record.Plain != "" || record.Instrumental) {
			return record, nil
		}
		params.Del("duration")
		params.Del("album_name")
		var choices []LyricsRecord
		if e = s.lyricsRequest(ctx, provider, "/api/search", params, &choices); e != nil {
			return empty, e
		}
		for _, r := range choices {
			if lyricsRecordingMatch(song, r.Title, r.Artist, r.Duration) {
				if validLRC(r.Synced, song.Duration) {
					return r, nil
				}
				if record.Plain == "" {
					record = r
				}
			}
		}
		if record.Plain != "" {
			return record, nil
		}
	case "qq":
		request := map[string]any{"comm": map[string]any{"ct": 19, "cv": 1859}, "music.search.SearchCgiService": map[string]any{"method": "DoSearchForQQMusicDesktop", "module": "music.search.SearchCgiService", "param": map[string]any{"num_per_page": 10, "page_num": 1, "query": song.Artist + " " + song.Title, "search_type": 0}}}
		b, _ := json.Marshal(request)
		var data map[string]json.RawMessage
		if e := s.lyricsRequest(ctx, provider, "/cgi-bin/musicu.fcg", url.Values{"data": {string(b)}}, &data); e != nil {
			return empty, e
		}
		var result struct {
			Data struct {
				Body struct {
					Song struct {
						List []struct {
							MID      string  `json:"mid"`
							Title    string  `json:"title"`
							Name     string  `json:"name"`
							Interval float64 `json:"interval"`
							Singer   []struct {
								Name string `json:"name"`
							} `json:"singer"`
						} `json:"list"`
					} `json:"song"`
				} `json:"body"`
			} `json:"data"`
		}
		json.Unmarshal(data["music.search.SearchCgiService"], &result)
		for _, candidate := range result.Data.Body.Song.List {
			artist := ""
			if len(candidate.Singer) > 0 {
				artist = candidate.Singer[0].Name
			}
			title := candidate.Title
			if title == "" {
				title = candidate.Name
			}
			if !lyricsRecordingMatch(song, title, artist, candidate.Interval) {
				continue
			}
			var lyrics struct {
				Code  int    `json:"retcode"`
				Lyric string `json:"lyric"`
			}
			params := url.Values{"songmid": {candidate.MID}, "format": {"json"}, "nobase64": {"1"}}
			if e := s.qqLyric(ctx, params, &lyrics); e != nil {
				continue
			}
			text := html.UnescapeString(lyrics.Lyric)
			if decoded, e := base64.StdEncoding.DecodeString(text); e == nil && strings.Contains(string(decoded), "[") {
				text = string(decoded)
			}
			if validLRC(text, song.Duration) {
				return LyricsRecord{Title: song.Title, Artist: song.Artist, Duration: candidate.Interval, Synced: text}, nil
			}
		}
	case "netease":
		for _, endpoint := range []string{"/api/search/get/web", "/api/cloudsearch/pc"} {
			var raw struct {
				Result json.RawMessage `json:"result"`
			}
			params := url.Values{"s": {song.Artist + " " + song.Title}, "type": {"1"}, "limit": {"8"}}
			if e := s.lyricsRequest(ctx, provider, endpoint, params, &raw); e != nil {
				continue
			}
			var result struct {
				Songs []struct {
					ID       int64   `json:"id"`
					Name     string  `json:"name"`
					Duration float64 `json:"duration"`
					DT       float64 `json:"dt"`
					Artists  []struct {
						Name string `json:"name"`
					} `json:"artists"`
					AR []struct {
						Name string `json:"name"`
					} `json:"ar"`
				} `json:"songs"`
			}
			if json.Unmarshal(raw.Result, &result) != nil {
				continue
			}
			for _, c := range result.Songs {
				artists := c.Artists
				if len(artists) == 0 {
					artists = c.AR
				}
				artist := ""
				if len(artists) > 0 {
					artist = artists[0].Name
				}
				duration := c.Duration / 1000
				if duration == 0 {
					duration = c.DT / 1000
				}
				if !lyricsRecordingMatch(song, c.Name, artist, duration) {
					continue
				}
				var data struct {
					YRC struct {
						Lyric string `json:"lyric"`
					} `json:"yrc"`
					LRC struct {
						Lyric string `json:"lyric"`
					} `json:"lrc"`
				}
				if e := s.lyricsRequest(ctx, provider, "/api/song/lyric", url.Values{"id": {strconv.FormatInt(c.ID, 10)}, "lv": {"-1"}, "tv": {"-1"}, "kv": {"-1"}, "yv": {"-1"}}, &data); e == nil {
					if record, err := decodeYRC(data.YRC.Lyric, song); err == nil {
						return record, nil
					}
					if !validLRC(data.LRC.Lyric, song.Duration) {
						continue
					}
					return LyricsRecord{Title: song.Title, Artist: song.Artist, Duration: duration, Synced: data.LRC.Lyric}, nil
				}
			}
		}
	case "kugou":
		var data struct {
			Candidates []struct {
				ID       string    `json:"id"`
				Access   string    `json:"accesskey"`
				Song     string    `json:"song"`
				Singer   string    `json:"singer"`
				Duration flexFloat `json:"duration"`
			} `json:"candidates"`
		}
		params := url.Values{"keyword": {song.Artist + " " + song.Title}, "duration": {fmt.Sprintf("%.0f", song.Duration*1000)}, "client": {"pc"}, "ver": {"1"}, "man": {"yes"}}
		if e := s.lyricsRequest(ctx, provider, "/search", params, &data); e != nil {
			return empty, e
		}
		for _, c := range data.Candidates {
			if !lyricsRecordingMatch(song, c.Song, c.Singer, float64(c.Duration)/1000) {
				continue
			}
			var lyrics struct {
				Content string `json:"content"`
			}
			params = url.Values{"id": {c.ID}, "accesskey": {c.Access}, "fmt": {"lrc"}, "charset": {"utf8"}, "client": {"pc"}, "ver": {"1"}}
			if e := s.lyricsRequest(ctx, provider, "/download", params, &lyrics); e != nil {
				continue
			}
			text, e := base64.StdEncoding.DecodeString(lyrics.Content)
			if e != nil {
				continue
			}
			if validLRC(string(text), song.Duration) {
				return LyricsRecord{Title: song.Title, Artist: song.Artist, Duration: float64(c.Duration) / 1000, Synced: string(text)}, nil
			}
		}
	case "musixmatch":
		s.mu.Lock()
		apiKey := s.musixmatchKey
		s.mu.Unlock()
		if apiKey == "" {
			return empty, errors.New("يحتاج مفتاح Musixmatch بصلاحية الكلمات المتزامنة")
		}
		params := url.Values{"apikey": {apiKey}, "q_track": {song.Title}, "q_artist": {song.Artist}, "f_has_subtitle": {"1"}, "page_size": {"8"}}
		var d struct {
			Message struct {
				Header struct {
					Code int `json:"status_code"`
				} `json:"header"`
				Body struct {
					List []struct {
						Track struct {
							ID       int64   `json:"track_id"`
							Title    string  `json:"track_name"`
							Artist   string  `json:"artist_name"`
							Duration float64 `json:"track_length"`
						} `json:"track"`
					} `json:"track_list"`
				} `json:"body"`
			} `json:"message"`
		}
		if e := s.lyricsRequest(ctx, provider, "/track.search", params, &d); e != nil {
			return empty, e
		}
		if d.Message.Header.Code != 200 {
			return empty, fmt.Errorf("Musixmatch status %d", d.Message.Header.Code)
		}
		for _, entry := range d.Message.Body.List {
			c := entry.Track
			if !lyricsRecordingMatch(song, c.Title, c.Artist, c.Duration) {
				continue
			}
			var sub struct {
				Message struct {
					Header struct {
						Code int `json:"status_code"`
					} `json:"header"`
					Body struct {
						Subtitle struct {
							Text string `json:"subtitle_body"`
						} `json:"subtitle"`
					} `json:"body"`
				} `json:"message"`
			}
			params = url.Values{"apikey": {apiKey}, "track_id": {strconv.FormatInt(c.ID, 10)}, "subtitle_format": {"lrc"}}
			if e := s.lyricsRequest(ctx, provider, "/track.subtitle.get", params, &sub); e == nil && sub.Message.Header.Code == 200 && validLRC(sub.Message.Body.Subtitle.Text, song.Duration) {
				return LyricsRecord{Title: song.Title, Artist: song.Artist, Duration: c.Duration, Synced: sub.Message.Body.Subtitle.Text}, nil
			}
		}
	default:
		return s.fetchAdditionalLyricProvider(ctx, song, provider)
	}
	return empty, errors.New("لم توجد كلمات متزامنة تطابق التسجيل")
}

type flexFloat float64

func (f *flexFloat) UnmarshalJSON(b []byte) error {
	v := strings.Trim(string(b), `"`)
	if v == "null" || v == "" {
		*f = 0
		return nil
	}
	n, e := strconv.ParseFloat(v, 64)
	*f = flexFloat(n)
	return e
}
func (s *Studio) qqLyric(ctx context.Context, params url.Values, target any) error {
	base := "https://c.y.qq.com"
	if s.lyricTestBases != nil && s.lyricTestBases["qq"] != "" {
		base = s.lyricTestBases["qq"]
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", base+"/lyric/fcgi-bin/fcg_query_lyric_new.fcg?"+params.Encode(), nil)
	req.Header.Set("Referer", "https://y.qq.com/portal/player.html")
	req.Header.Set("User-Agent", "Mozilla/5.0")
	res, e := s.client.Do(req)
	if e != nil {
		return e
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("QQ HTTP %d", res.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(target)
}
func (s *Studio) remoteLyricsFrom(ctx context.Context, song Song, provider string) (LyricsRecord, error) {
	ctx, cancelAll := context.WithTimeout(ctx, 40*time.Second)
	defer cancelAll()
	automatic := provider == "" || provider == "auto"
	bestSelection := automatic || provider == "auto-refresh"
	if provider == "" || provider == "auto" {
		if cached, e := s.readLyricCache(song); e == nil && (cached.Record.Synced != "" || cached.Record.Plain != "" || cached.Record.Instrumental) {
			r := cached.Record
			r.Words = cached.Words
			r.Cached = true
			return r, nil
		}
	}
	providers := s.GetState().Preferences.LyricProviders
	if len(providers) == 0 {
		providers = defaultLyricProviders
	}
	if provider != "" && provider != "auto" && provider != "auto-refresh" {
		providers = []string{provider}
	}
	if bestSelection {
		providers = preferredLyricProviders(providers)
	}
	prefetched := map[string]<-chan lyricFetchResult{}
	if bestSelection {
		prefetched = s.prefetchWordLyrics(ctx, song, providers)
	}
	var fallback LyricsRecord
	bestRank := 0
	var failures []string
	for _, p := range providers {
		if bestSelection && !wordLyricProvider(p) && bestRank >= 2 {
			break
		}
		if ctx.Err() != nil {
			if bestRank > 0 {
				break
			}
			return fallback, ctx.Err()
		}
		var r LyricsRecord
		var e error
		if result, ok := prefetched[p]; ok {
			select {
			case value := <-result:
				r, e = value.record, value.err
			case <-ctx.Done():
				e = ctx.Err()
			}
		} else {
			run, cancel := context.WithTimeout(ctx, 12*time.Second)
			r, e = s.fetchLyricProvider(run, song, p)
			cancel()
		}
		if e != nil {
			failures = append(failures, lyricProviderNames[p]+": "+e.Error())
			continue
		}
		r.Provider = p
		if r.Instrumental {
			r.Synced = ""
			r.Plain = ""
			r.Words = nil
		}
		rank := lyricRecordRank(r, song)
		if rank < 2 {
			r.Synced = ""
			r.Words = nil
		}
		if rank > bestRank || rank == 1 && fallback.Instrumental && r.Plain != "" {
			fallback = r
			bestRank = rank
		}
		if rank == 4 || !bestSelection && rank > 0 || !wordLyricProvider(p) && rank >= 2 {
			return s.cacheRemoteLyrics(song, r, automatic)
		}
	}
	if bestRank > 0 {
		return s.cacheRemoteLyrics(song, fallback, automatic)
	}
	return fallback, errors.New(strings.Join(failures, "؛ "))
}

// A late automatic fetch must not replace words/timing edited while it ran.
func (s *Studio) cacheRemoteLyrics(song Song, record LyricsRecord, automatic bool) (LyricsRecord, error) {
	s.translationMu.Lock()
	defer s.translationMu.Unlock()
	if automatic {
		if cache, err := s.readLyricCache(song); err == nil && (cache.Record.Synced != "" || cache.Record.Plain != "" || cache.Record.Instrumental) {
			record = cache.Record
			record.Words = cache.Words
			record.Cached = true
			return record, nil
		}
	}
	if err := s.writeLyricCache(LyricCache{Song: song, Record: record, Words: record.Words}); err != nil {
		return record, fmt.Errorf("الكلمات جُلبت ولكن تعذر حفظها: %w", err)
	}
	return record, nil
}
func (s *Studio) FetchLyricsFrom(id, provider string) (LyricView, error) {
	t, ok := s.task(id)
	if !ok {
		return LyricView{}, errors.New("اختر مسارًا")
	}
	r, e := s.remoteLyricsFrom(context.Background(), t.Song, provider)
	if e != nil {
		return LyricView{}, e
	}
	return s.persistLyricView(t, r)
}
func (s *Studio) persistLyricView(t Task, r LyricsRecord) (LyricView, error) {
	text := r.Plain
	timed := false
	ext := ".txt"
	if validLRC(r.Synced, t.Song.Duration) {
		text = r.Synced
		timed = true
		ext = ".lrc"
	}
	if text == "" {
		return LyricView{Status: "مقطوعة بلا كلمات أو كلمات غير متاحة"}, nil
	}
	cached, e := s.readLyricCache(t.Song)
	status := lyricProviderNames[r.Provider]
	if status == "" {
		status = r.Provider
	}
	if r.Cached {
		status += " · محفوظ محليًا"
	}
	if t.Path != "" {
		path := strings.TrimSuffix(t.Path, filepath.Ext(t.Path)) + ext
		if e := writeChanged(path, []byte(text)); e != nil {
			status += " · الكاش محفوظ؛ تعذر حفظ الملف الجانبي"
		} else {
			s.edit(t.ID, func(j *Task) { j.Lyrics = path; j.LyricsSource = r.Provider })
			s.mu.Lock()
			s.save()
			s.mu.Unlock()
		}
	}
	view := LyricView{Text: text, Timed: timed, Status: status, Source: r.Provider}
	if p := s.GetState().Preferences; p.AutoTranslate {
		_ = s.StartTranslation(t.ID, p.TranslationTarget, p.TranslationSource, false)
	}
	if e == nil {
		view.Words = cached.Words
		view.Pronunciation = cached.Pronunciation
		view.Alignment = cached.Alignment
	}
	if t.Path != "" && s.GetState().Preferences.AutoAlign && !wordTimingComplete(view.Words, text, t.Song.Duration) {
		s.autoAlign(t.ID)
	}
	return view, nil
}
func (s *Studio) CacheLibraryLyrics() (int, error) {
	count := 0
	for _, t := range s.GetState().Jobs {
		if t.Status != "done" {
			continue
		}
		if s.ctx != nil && s.ctx.Err() != nil {
			break
		}
		if _, e := s.GetLyrics(t.ID, false); e == nil {
			count++
		}
	}
	return count, nil
}
