package main

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

var extraSources = []string{"ytmusic", "audius", "hearthis", "mixcloud", "jamendo", "audiomack", "reverbnation", "commons"}
var extraSourceSites = map[string]string{"ytmusic": "music.youtube.com", "audius": "audius.co", "hearthis": "hearthis.at", "mixcloud": "mixcloud.com", "jamendo": "jamendo.com", "audiomack": "audiomack.com", "reverbnation": "reverbnation.com", "commons": "commons.wikimedia.org"}

func sourceForHost(host string) string {
	host = strings.TrimPrefix(strings.ToLower(host), "www.")
	if host == "youtube.com" || host == "youtu.be" || host == "music.youtube.com" {
		return "ytmusic"
	}
	for p, site := range extraSourceSites {
		if host == site {
			return p
		}
	}
	return ""
}
func extractorSource(provider string) bool {
	switch provider {
	case "soundcloud", "bandcamp", "ytmusic", "hearthis", "mixcloud", "jamendo", "audiomack", "reverbnation", "audius":
		return true
	}
	return false
}
func (s *Studio) providerSearch(ctx context.Context, provider, q string) (Result, error) {
	switch provider {
	case "soundcloud":
		return s.extract(ctx, "scsearch20:"+q, true, provider)
	case "bandcamp":
		return s.bandcampSearch(ctx, q)
	case "archive":
		return s.archiveSearch(ctx, q)
	case "tidal":
		return s.tidalSearch(ctx, q)
	case "spotify":
		return s.spotifySearch(ctx, q)
	case "ytmusic":
		return s.extract(ctx, "https://music.youtube.com/search?q="+url.QueryEscape(q), true, provider)
	case "audius":
		return s.audiusSearch(ctx, q)
	case "hearthis":
		return s.hearthisSearch(ctx, q)
	case "mixcloud":
		return s.mixcloudSearch(ctx, q)
	case "commons":
		return s.commonsSearch(ctx, q, "")
	case "jamendo", "audiomack", "reverbnation":
		return s.indexedMusicSearch(ctx, q, provider)
	}
	return Result{}, errors.New("مصدر بحث غير معروف")
}
func (s *Studio) sourceEndpoint(provider, base string) string {
	if s.sourceTestBases != nil && s.sourceTestBases[provider] != "" {
		return s.sourceTestBases[provider]
	}
	return base
}
func newMusicResult(q string) Result { return Result{Title: q, Tracks: []Song{}, Complete: false} }
func (s *Studio) audiusSearch(ctx context.Context, q string) (Result, error) {
	base := s.sourceEndpoint("audius", "https://discoveryprovider.audius.co")
	var data struct {
		Data []struct {
			ID            string         `json:"id"`
			Title         string         `json:"title"`
			Genre         string         `json:"genre"`
			Duration      float64        `json:"duration"`
			Permalink     string         `json:"permalink"`
			Artwork       map[string]any `json:"artwork"`
			Streamable    bool           `json:"is_streamable"`
			StreamGated   bool           `json:"is_stream_gated"`
			Downloadable  bool           `json:"is_downloadable"`
			DownloadGated bool           `json:"is_download_gated"`
			User          struct {
				Name string `json:"name"`
			} `json:"user"`
		} `json:"data"`
	}
	r := newMusicResult(q)
	if e := s.jsonGet(ctx, base+"/v1/tracks/search?query="+url.QueryEscape(q)+"&limit=20&app_name=MadarMusic", &data); e != nil {
		return r, e
	}
	for _, v := range data.Data {
		if !v.Streamable || v.StreamGated {
			continue
		}
		art, _ := v.Artwork["1000x1000"].(string)
		if art == "" {
			art, _ = v.Artwork["480x480"].(string)
		}
		link := "https://audius.co" + v.Permalink
		original := ""
		if v.Downloadable && !v.DownloadGated {
			original = base + "/v1/tracks/" + url.PathEscape(v.ID) + "/download?app_name=MadarMusic"
		}
		r.Tracks = append(r.Tracks, Song{Genres: cleanGenres([]string{v.Genre}), ID: key("audius" + v.ID), Title: v.Title, Artist: v.User.Name, Duration: v.Duration, Artwork: art, Provider: "audius", URL: link, AudioURL: base + "/v1/tracks/" + url.PathEscape(v.ID) + "/stream?app_name=MadarMusic", OriginalURL: original, Ref: v.ID})
	}
	r.Total = len(r.Tracks)
	r.Notice = "Audius: بث عام؛ الملف الأصلي يُحاول فقط عندما يتيحه الفنان دون بوابة دخول."
	return r, nil
}
func (s *Studio) hearthisSearch(ctx context.Context, q string) (Result, error) {
	base := s.sourceEndpoint("hearthis", "https://api-v2.hearthis.at")
	var data []struct {
		ID       string    `json:"id"`
		Title    string    `json:"title"`
		Genre    string    `json:"genre"`
		Duration flexFloat `json:"duration"`
		Artwork  string    `json:"artwork_url"`
		Thumb    string    `json:"thumb"`
		URL      string    `json:"permalink_url"`
		Stream   string    `json:"stream_url"`
		Private  string    `json:"private"`
		User     struct {
			Name string `json:"username"`
		} `json:"user"`
	}
	r := newMusicResult(q)
	if e := s.jsonGet(ctx, base+"/search/?type=tracks&t="+url.QueryEscape(q)+"&count=20", &data); e != nil {
		return r, e
	}
	for _, v := range data {
		if v.Private == "1" || v.URL == "" {
			continue
		}
		art := v.Artwork
		if art == "" {
			art = v.Thumb
		}
		r.Tracks = append(r.Tracks, Song{Genres: cleanGenres([]string{v.Genre}), ID: key("hearthis" + v.ID), Title: v.Title, Artist: v.User.Name, Duration: float64(v.Duration), Artwork: art, Provider: "hearthis", URL: v.URL, AudioURL: v.Stream, Ref: v.ID})
	}
	r.Total = len(r.Tracks)
	return r, nil
}
func (s *Studio) mixcloudSearch(ctx context.Context, q string) (Result, error) {
	var data struct {
		Data []struct {
			Key      string            `json:"key"`
			URL      string            `json:"url"`
			Name     string            `json:"name"`
			Length   float64           `json:"audio_length"`
			Pictures map[string]string `json:"pictures"`
			User     struct {
				Name string `json:"name"`
			} `json:"user"`
		} `json:"data"`
	}
	r := newMusicResult(q)
	if e := s.jsonGet(ctx, s.sourceEndpoint("mixcloud", "https://api.mixcloud.com")+"/search/?q="+url.QueryEscape(q)+"&type=cloudcast&limit=20", &data); e != nil {
		return r, e
	}
	for _, v := range data.Data {
		art := v.Pictures["extra_large"]
		if art == "" {
			art = v.Pictures["large"]
		}
		r.Tracks = append(r.Tracks, Song{ID: key("mixcloud" + v.Key), Title: v.Name, Artist: v.User.Name, Duration: v.Length, Artwork: art, Provider: "mixcloud", URL: v.URL, Ref: v.Key})
	}
	r.Total = len(r.Tracks)
	r.Notice = "Mixcloud: عروض وميكسات. لا يُستبدل بها تسجيل أغنية مختلف عند المطابقة."
	return r, nil
}
func plainMetadata(text string) string {
	return strings.TrimSpace(html.UnescapeString(regexp.MustCompile(`<[^>]*>`).ReplaceAllString(text, "")))
}
func (s *Studio) commonsSearch(ctx context.Context, q, title string) (Result, error) {
	params := url.Values{"action": {"query"}, "format": {"json"}, "prop": {"imageinfo"}, "iiprop": {"url|size|mime|extmetadata"}}
	if title != "" {
		params.Set("titles", title)
	} else {
		params.Set("generator", "search")
		params.Set("gsrsearch", "filetype:audio "+q)
		params.Set("gsrnamespace", "6")
		params.Set("gsrlimit", "20")
	}
	var data struct {
		Query struct {
			Pages map[string]struct {
				Title string `json:"title"`
				Info  []struct {
					URL      string  `json:"url"`
					Page     string  `json:"descriptionurl"`
					Thumb    string  `json:"thumburl"`
					Size     int64   `json:"size"`
					MIME     string  `json:"mime"`
					Duration float64 `json:"duration"`
					Metadata map[string]struct {
						Value any `json:"value"`
					} `json:"extmetadata"`
				} `json:"imageinfo"`
			} `json:"pages"`
		} `json:"query"`
	}
	r := newMusicResult(q)
	if e := s.jsonGet(ctx, s.sourceEndpoint("commons", "https://commons.wikimedia.org")+"/w/api.php?"+params.Encode(), &data); e != nil {
		return r, e
	}
	for id, page := range data.Query.Pages {
		if len(page.Info) == 0 {
			continue
		}
		v := page.Info[0]
		if !strings.HasPrefix(v.MIME, "audio/") {
			continue
		}
		name := plainMetadata(fmt.Sprint(v.Metadata["ObjectName"].Value))
		if name == "" || name == "<nil>" {
			name = strings.TrimSuffix(strings.TrimPrefix(page.Title, "File:"), filepath.Ext(page.Title))
		}
		artist := plainMetadata(fmt.Sprint(v.Metadata["Artist"].Value))
		if artist == "<nil>" {
			artist = "Wikimedia Commons"
		}
		r.Tracks = append(r.Tracks, Song{ID: key("commons" + id), Title: name, Artist: artist, Duration: v.Duration, Artwork: v.Thumb, Provider: "commons", URL: v.Page, AudioURL: v.URL, Size: v.Size, Ref: id})
	}
	r.Total = len(r.Tracks)
	return r, nil
}
func indexedSourceLink(provider, address string) bool {
	u, e := url.Parse(address)
	if e != nil || sourceForHost(u.Hostname()) != provider {
		return false
	}
	switch provider {
	case "jamendo":
		return regexp.MustCompile(`^/track/\d+`).MatchString(u.Path)
	case "audiomack":
		return strings.Contains(u.Path, "/song/")
	case "reverbnation":
		return strings.Contains(u.Path, "/song/")
	}
	return false
}
func (s *Studio) indexedMusicSearch(ctx context.Context, q, provider string) (Result, error) {
	site := extraSourceSites[provider]
	suffix := "/song/"
	if provider == "jamendo" {
		suffix = "/track/"
	}
	endpoint := s.sourceEndpoint("index", "https://www.bing.com") + "/search?format=rss&q=" + url.QueryEscape("site:"+site+" inurl:"+strings.Trim(suffix, "/")+" "+q)
	req, _ := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	req.Header.Set("User-Agent", "MadarMusic/1.0")
	res, e := s.client.Do(req)
	r := newMusicResult(q)
	if e != nil {
		return r, e
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return r, fmt.Errorf("بحث الفهرس HTTP %d", res.StatusCode)
	}
	var feed struct {
		Channel struct {
			Items []struct {
				Link string `xml:"link"`
			} `xml:"item"`
		} `xml:"channel"`
	}
	if e = xml.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(&feed); e != nil {
		return r, e
	}
	seen := map[string]bool{}
	failures := 0
	for _, item := range feed.Channel.Items {
		if seen[item.Link] || !indexedSourceLink(provider, item.Link) {
			continue
		}
		seen[item.Link] = true
		if len(seen) > 4 {
			break
		}
		data, e := s.extract(ctx, item.Link, true, provider)
		if e == nil {
			r.Tracks = append(r.Tracks, data.Tracks...)
		} else {
			failures++
		}
	}
	r.Total = len(r.Tracks)
	r.Notice = "بحث في الصفحات العامة المفهرسة؛ نتائج جزئية. جودة الملف وصلاحية الوصول تُفحصان لكل مسار."
	if len(r.Tracks) == 0 && failures > 0 {
		return r, errors.New("عُثر على صفحات لكن المصدر لم يتيح بيانات/صوتًا قابلًا للقراءة")
	}
	return r, nil
}
