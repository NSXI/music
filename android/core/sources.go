package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

func validWebURL(v string) error {
	u, e := url.Parse(v)
	if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
		return errors.New("استخدم رابط HTTP أو HTTPS صالحًا")
	}
	return nil
}
func (s *Studio) jsonGet(ctx context.Context, address string, target any) error {
	if e := validWebURL(address); e != nil {
		return e
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", address, nil)
	req.Header.Set("User-Agent", "MadarMusic/1.0 (desktop music library)")
	r, e := s.client.Do(req)
	if e != nil {
		return e
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", r.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(r.Body, 24<<20)).Decode(target)
}
func (s *Studio) Search(query, provider string) (Result, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return Result{}, errors.New("ألصق رابطًا أو اكتب الفنان والأغنية")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	var r Result
	var e error
	if strings.HasPrefix(query, "spotify:") {
		bits := strings.Split(query, ":")
		if len(bits) == 3 {
			query = "https://open.spotify.com/" + bits[1] + "/" + bits[2]
		}
	}
	u, _ := url.Parse(query)
	host := strings.ToLower(u.Hostname())
	switch {
	case host == "open.spotify.com":
		r, e = s.spotify(ctx, u)
	case host == "soundcloud.com" || host == "www.soundcloud.com" || host == "on.soundcloud.com":
		r, e = s.extract(ctx, query, true, "soundcloud")
	case strings.HasSuffix(host, ".bandcamp.com"):
		r, e = s.extract(ctx, query, true, "bandcamp")
	case host == "archive.org" || host == "www.archive.org":
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) >= 2 && (parts[0] == "details" || parts[0] == "download") {
			r, e = s.archiveItem(ctx, parts[1])
		} else {
			e = errors.New("ألصق رابط عنصر من archive.org/details")
		}
	case host == "tidal.com" || host == "www.tidal.com" || host == "listen.tidal.com":
		r, e = s.tidalLink(ctx, u)
	case sourceForHost(host) != "" && sourceForHost(host) != "commons":
		r, e = s.extract(ctx, query, true, sourceForHost(host))
	case host == "commons.wikimedia.org":
		title, _ := url.PathUnescape(strings.TrimPrefix(u.Path, "/wiki/"))
		r, e = s.commonsSearch(ctx, title, title)
	case host != "":
		if e = validWebURL(query); e == nil {
			title, _ := url.PathUnescape(filepath.Base(u.Path))
			r = Result{Title: title, Complete: true, Total: 1, Tracks: []Song{{ID: key(query), Title: strings.TrimSuffix(title, filepath.Ext(title)), Artist: "ملف مباشر", Album: "تنزيلات مباشرة", Provider: "direct", URL: query, AudioURL: query}}}
		}
	default:
		switch provider {
		case "all":
			r, e = s.searchAll(ctx, query)
		case "soundcloud":
			r, e = s.extract(ctx, "scsearch20:"+query, true, "soundcloud")
		case "bandcamp":
			r, e = s.bandcampSearch(ctx, query)
		case "tidal":
			r, e = s.tidalSearch(ctx, query)
		case "archive":
			r, e = s.archiveSearch(ctx, query)
		case "spotify":
			r, e = s.spotifySearch(ctx, query)
		default:
			r, e = s.providerSearch(ctx, provider, query)
		}
	}
	if e != nil {
		return r, e
	}
	return s.remember(r), nil
}

type extractorInfo struct {
	ID         string  `json:"id"`
	Title      string  `json:"title"`
	Track      string  `json:"track"`
	Artist     string  `json:"artist"`
	Uploader   string  `json:"uploader"`
	Album      string  `json:"album"`
	Duration   float64 `json:"duration"`
	Thumbnail  string  `json:"thumbnail"`
	Thumbnails []struct {
		URL   string `json:"url"`
		Width int    `json:"width"`
	} `json:"thumbnails"`
	WebURL        string           `json:"webpage_url"`
	URL           string           `json:"url"`
	Entries       []*extractorInfo `json:"entries"`
	PlaylistCount int              `json:"playlist_count"`
	Genres        []string         `json:"genres"`
	Genre         string           `json:"genre"`
	Availability  string           `json:"availability"`
}

func (s *Studio) extract(ctx context.Context, address string, flat bool, provider string) (Result, error) {
	args := []string{"--ignore-config", "--no-plugin-dirs", "--no-cache-dir", "--no-warnings", "--socket-timeout", "20", "--retries", "1", "--dump-single-json", "--ignore-errors"}
	if flat {
		args = append(args, "--flat-playlist")
	}
	args = append(args, "--", address)
	cmd := s.command(ctx, "yt-dlp", args...)
	var stdout, stderr limitedBuffer
	stdout.Limit = 32 << 20
	stderr.Limit = 4096
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		return Result{}, ctx.Err()
	}
	var info extractorInfo
	if json.Unmarshal(stdout.Bytes(), &info) != nil {
		if err != nil {
			return Result{}, fmt.Errorf("تعذر قراءة المصدر: %s", trimError(stderr.Bytes()))
		}
		return Result{}, errors.New("لم يرجع المصدر بيانات قابلة للقراءة")
	}
	r := Result{Title: info.Title, Tracks: []Song{}, Complete: err == nil, Total: info.PlaylistCount}
	missing := 0
	var add func(*extractorInfo)
	add = func(v *extractorInfo) {
		if v == nil {
			missing++
			return
		}
		if v.Entries != nil {
			for _, c := range v.Entries {
				add(c)
			}
			return
		}
		title := v.Track
		if title == "" {
			title = v.Title
		}
		artist := v.Artist
		if artist == "" {
			artist = v.Uploader
		}
		link := v.WebURL
		if link == "" {
			link = v.URL
		}
		if link == "" && provider == "soundcloud" && v.ID != "" {
			link = "https://api.soundcloud.com/tracks/" + v.ID
		}
		if title == "" || link == "" {
			missing++
			return
		}
		artwork := extractorArtwork(provider, v)
		r.Tracks = append(r.Tracks, Song{Genres: cleanGenres(append(v.Genres, v.Genre)), ID: key(provider + v.ID + link), Title: title, Artist: artist, Album: v.Album, Duration: v.Duration, Artwork: artwork, Provider: provider, URL: link, Ref: v.ID})
	}
	add(&info)
	if r.Total == 0 {
		r.Total = len(r.Tracks) + missing
	}
	r.Notice = fmt.Sprintf("جُلب %d مسارًا. يتم فحص الصوت والجودة عند التنزيل.", len(r.Tracks))
	if missing > 0 || err != nil {
		r.Complete = false
		r.Notice += fmt.Sprintf(" يوجد %d عنصر غير متاح أو خطأ في بعض العناصر.", missing)
	}
	return r, nil
}
func (s *Studio) bandcampSearch(ctx context.Context, q string) (Result, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://bandcamp.com/search?item_type=t&q="+url.QueryEscape(q), nil)
	res, e := s.client.Do(req)
	if e != nil {
		return Result{}, e
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return Result{}, fmt.Errorf("Bandcamp HTTP %d", res.StatusCode)
	}
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 3<<20))
	links := regexp.MustCompile(`https://[a-zA-Z0-9-]+\.bandcamp\.com/track/[a-zA-Z0-9_-]+`).FindAllString(string(raw), -1)
	r := Result{Title: q, Tracks: []Song{}, Complete: false}
	seen := map[string]bool{}
	for _, link := range links {
		if seen[link] {
			continue
		}
		seen[link] = true
		if len(r.Tracks) >= 6 {
			break
		}
		item, e := s.extract(ctx, link, true, "bandcamp")
		if e == nil {
			r.Tracks = append(r.Tracks, item.Tracks...)
		}
	}
	r.Total = len(r.Tracks)
	r.Notice = "نتائج Bandcamp. جودة البث قد تكون MP3؛ الروابط الأصلية المسموحة تُحفظ بصيغتها الفعلية."
	return r, nil
}

type archiveMeta struct {
	Metadata struct {
		Title      string   `json:"title"`
		Creator    any      `json:"creator"`
		Album      string   `json:"album"`
		NoDownload flexBool `json:"nodownload"`
	} `json:"metadata"`
	Files []struct {
		Name    string    `json:"name"`
		Size    flexFloat `json:"size"`
		Title   string    `json:"title"`
		Creator string    `json:"creator"`
		Format  string    `json:"format"`
		Length  string    `json:"length"`
		Private flexBool  `json:"private"`
		Bitrate string    `json:"bitrate"`
	} `json:"files"`
}

func (s *Studio) archiveItem(ctx context.Context, id string) (Result, error) {
	var m archiveMeta
	if e := s.jsonGet(ctx, "https://archive.org/metadata/"+url.PathEscape(id), &m); e != nil {
		return Result{}, e
	}
	if bool(m.Metadata.NoDownload) {
		return Result{}, errors.New("التنزيل غير متاح لهذا العنصر")
	}
	r := Result{Title: m.Metadata.Title, Tracks: []Song{}, Complete: true}
	artist, _ := m.Metadata.Creator.(string)
	if a, ok := m.Metadata.Creator.([]any); ok && len(a) > 0 {
		artist, _ = a[0].(string)
	}
	cover := "https://archive.org/services/img/" + url.PathEscape(id)
	best := map[string]Song{}
	ranks := map[string]int{}
	for _, f := range m.Files {
		ext := strings.ToLower(filepath.Ext(f.Name))
		rank := 0
		switch ext {
		case ".flac":
			rank = 100
		case ".wav", ".aiff":
			rank = 95
		case ".m4a":
			rank = 60
		case ".opus":
			rank = 55
		case ".ogg":
			rank = 50
		case ".mp3":
			rank = 40
		default:
			continue
		}
		if bool(f.Private) {
			continue
		}
		title := f.Title
		if title == "" {
			title = strings.TrimSuffix(filepath.Base(f.Name), filepath.Ext(f.Name))
		}
		a := f.Creator
		if a == "" {
			a = artist
		}
		dur := parseDuration(f.Length)
		k := norm(title)
		if old, ok := ranks[k]; ok && old >= rank {
			continue
		}
		ranks[k] = rank
		audio := "https://archive.org/download/" + url.PathEscape(id) + "/" + strings.ReplaceAll(url.PathEscape(f.Name), "%2F", "/")
		advertisedBitrate, _ := strconv.Atoi(f.Bitrate)
		best[k] = Song{Size: int64(f.Size), Codec: strings.ToUpper(strings.TrimPrefix(ext, ".")), Bitrate: advertisedBitrate * 1000, ID: key(audio), Title: title, Artist: a, Album: m.Metadata.Title, Duration: dur, Artwork: cover, Provider: "archive", URL: "https://archive.org/details/" + id, AudioURL: audio, Ref: id}
	}
	for _, v := range best {
		r.Tracks = append(r.Tracks, v)
	}
	sort.Slice(r.Tracks, func(i, j int) bool { return r.Tracks[i].Title < r.Tracks[j].Title })
	r.Total = len(r.Tracks)
	r.Notice = "اختيرت أعلى صيغة متاحة لكل عنوان في عنصر Internet Archive. تُفحص الجودة بعد التنزيل."
	return r, nil
}
func (s *Studio) archiveSearch(ctx context.Context, q string) (Result, error) {
	clean := strings.ReplaceAll(q, `"`, "")
	params := url.Values{"q": {`mediatype:audio AND (` + clean + `)`}, "output": {"json"}, "rows": {"8"}, "fl[]": {"identifier", "title"}}
	var data struct {
		Response struct {
			Docs []struct {
				ID    string `json:"identifier"`
				Title string `json:"title"`
			} `json:"docs"`
		} `json:"response"`
	}
	if e := s.jsonGet(ctx, "https://archive.org/advancedsearch.php?"+params.Encode(), &data); e != nil {
		return Result{}, e
	}
	r := Result{Title: q, Tracks: []Song{}, Complete: false}
	for _, d := range data.Response.Docs {
		if ctx.Err() != nil {
			break
		}
		a, e := s.archiveItem(ctx, d.ID)
		if e == nil {
			r.Tracks = append(r.Tracks, a.Tracks...)
		}
	}
	r.Total = len(r.Tracks)
	r.Notice = "نتائج صوتية من Internet Archive. راجع التسجيل والفنان قبل الإضافة."
	return r, nil
}

type tidalTrack struct {
	ID       json.Number `json:"id"`
	Title    string      `json:"title"`
	Version  string      `json:"version"`
	Duration float64     `json:"duration"`
	ISRC     string      `json:"isrc"`
	Artist   struct {
		Name string `json:"name"`
	} `json:"artist"`
	Artists []struct {
		Name string `json:"name"`
	} `json:"artists"`
	Album struct {
		Title string `json:"title"`
		Cover string `json:"cover"`
	} `json:"album"`
}

func fromTidal(t tidalTrack) Song {
	artist := t.Artist.Name
	if artist == "" && len(t.Artists) > 0 {
		artist = t.Artists[0].Name
	}
	title := t.Title
	if t.Version != "" && !strings.Contains(strings.ToLower(title), strings.ToLower(t.Version)) {
		title += " (" + t.Version + ")"
	}
	cover := ""
	if t.Album.Cover != "" {
		cover = "https://resources.tidal.com/images/" + strings.ReplaceAll(t.Album.Cover, "-", "/") + "/640x640.jpg"
	}
	return Song{ID: "tidal-" + t.ID.String(), Title: title, Artist: artist, Album: t.Album.Title, Duration: t.Duration, Artwork: cover, Provider: "tidal", Ref: t.ID.String(), URL: "https://tidal.com/browse/track/" + t.ID.String(), ISRC: t.ISRC}
}
func (s *Studio) tidalBase() (string, error) {
	p := s.GetState().Preferences
	if p.TidalURL == "" {
		return "", errors.New("أضف عنوان خادم TIDAL المتوافق في الإعدادات؛ الخوادم العامة التي اختُبرت غير متاحة")
	}
	return p.TidalURL, nil
}
func (s *Studio) tidalSearch(ctx context.Context, q string) (Result, error) {
	base, e := s.tidalBase()
	if e != nil {
		return Result{}, e
	}
	var raw json.RawMessage
	if e = s.jsonGet(ctx, base+"/search/?s="+url.QueryEscape(q), &raw); e != nil {
		return Result{}, e
	}
	var wrap struct {
		Data json.RawMessage `json:"data"`
	}
	json.Unmarshal(raw, &wrap)
	if len(wrap.Data) > 0 {
		raw = wrap.Data
	}
	var list struct {
		Tracks struct {
			Items []tidalTrack `json:"items"`
		} `json:"tracks"`
	}
	if e = json.Unmarshal(raw, &list); e != nil {
		return Result{}, e
	}
	r := Result{Title: q, Tracks: []Song{}, Complete: false}
	for _, t := range list.Tracks.Items {
		r.Tracks = append(r.Tracks, fromTidal(t))
	}
	r.Total = len(r.Tracks)
	r.Notice = "بيانات TIDAL عبر الخادم الذي أعددته. توفر الملف والجودة يُفحصان عند التنزيل."
	return r, nil
}
func (s *Studio) tidalLink(ctx context.Context, u *url.URL) (Result, error) {
	m := regexp.MustCompile(`/(track|album|playlist)/([a-zA-Z0-9-]+)`).FindStringSubmatch(u.Path)
	if len(m) != 3 {
		return Result{}, errors.New("رابط TIDAL غير معروف")
	}
	base, e := s.tidalBase()
	if e != nil {
		return Result{}, e
	}
	endpoint := "/info/?id=" + m[2]
	if m[1] != "track" {
		endpoint = "/" + m[1] + "/?id=" + m[2]
	}
	var raw json.RawMessage
	if e = s.jsonGet(ctx, base+endpoint, &raw); e != nil {
		return Result{}, e
	}
	var wrap struct {
		Data json.RawMessage `json:"data"`
	}
	json.Unmarshal(raw, &wrap)
	if len(wrap.Data) > 0 {
		raw = wrap.Data
	}
	r := Result{Title: "TIDAL", Tracks: []Song{}, Complete: m[1] == "track"}
	if m[1] == "track" {
		var t tidalTrack
		if e = json.Unmarshal(raw, &t); e != nil {
			return r, e
		}
		if t.Title == "" {
			return r, errors.New("لم يرجع الخادم معلومات الأغنية")
		}
		r.Tracks = append(r.Tracks, fromTidal(t))
		r.Title = t.Title
	} else {
		var data struct {
			Title string `json:"title"`
			Items []struct {
				Item tidalTrack `json:"item"`
			} `json:"items"`
		}
		json.Unmarshal(raw, &data)
		r.Title = data.Title
		for _, v := range data.Items {
			if v.Item.Title != "" {
				r.Tracks = append(r.Tracks, fromTidal(v.Item))
			}
		}
		r.Notice = "تأكد من عدد العناصر؛ بعض الخوادم ترجع صفحة واحدة فقط."
	}
	r.Total = len(r.Tracks)
	return r, nil
}
func (s *Studio) tidalAudio(ctx context.Context, song Song) (string, error) {
	base, e := s.tidalBase()
	if e != nil {
		return "", e
	}
	for _, quality := range []string{"HI_RES_LOSSLESS", "LOSSLESS", "HIGH"} {
		var raw json.RawMessage
		if e = s.jsonGet(ctx, base+"/track/?id="+url.QueryEscape(song.Ref)+"&quality="+quality, &raw); e != nil {
			continue
		}
		var w struct {
			Data json.RawMessage `json:"data"`
		}
		json.Unmarshal(raw, &w)
		if len(w.Data) > 0 {
			raw = w.Data
		}
		var info struct {
			Manifest         string `json:"manifest"`
			OriginalTrackURL string `json:"OriginalTrackUrl"`
		}
		json.Unmarshal(raw, &info)
		manifest, er := base64.StdEncoding.DecodeString(info.Manifest)
		if er == nil && len(manifest) > 0 {
			var b struct {
				Encryption string   `json:"encryptionType"`
				URLs       []string `json:"urls"`
			}
			if json.Unmarshal(manifest, &b) == nil {
				if b.Encryption != "" && b.Encryption != "NONE" {
					e = errors.New("الملف محمي؛ جارٍ الانتقال لمصدر آخر")
					continue
				}
				if len(b.URLs) > 0 && validWebURL(b.URLs[0]) == nil {
					return b.URLs[0], nil
				}
			} else {
				return s.saveClearDASH(manifest)
			}
		}
		if info.OriginalTrackURL != "" && validWebURL(info.OriginalTrackURL) == nil {
			return info.OriginalTrackURL, nil
		}
	}
	if e == nil {
		e = errors.New("لا يوجد رابط صوت غير محمي لدى الخادم")
	}
	return "", e
}

func (s *Studio) resolve(ctx context.Context, song Song, p Preferences) ([]Song, error) {
	items := []Song{song}
	if !p.AutoMatch || song.Provider == "direct" {
		return items, nil
	}
	for _, provider := range append([]string{"tidal", "archive", "bandcamp", "soundcloud"}, extraSources...) {
		if provider == song.Provider {
			continue
		}
		if provider == "tidal" && p.TidalURL == "" {
			continue
		}
		c := song
		c.Provider = provider
		c.Ref = ""
		c.AudioURL = ""
		c.URL = "search:" + song.Artist + " " + song.Title
		items = append(items, c)
	}
	return items, nil
}
func (s *Studio) fetchCandidate(ctx context.Context, t Task, c Song, p Preferences) (string, error) {
	if strings.HasPrefix(c.URL, "search:") {
		q := strings.TrimPrefix(c.URL, "search:")
		var r Result
		var e error
		r, e = s.providerSearch(ctx, c.Provider, q)
		if e != nil {
			return "", e
		}
		matches := []Song{}
		for _, candidate := range r.Tracks {
			if sameSong(c, candidate) {
				matches = append(matches, candidate)
			}
		}
		if len(matches) == 0 {
			return "", errors.New("لم توجد نسخة تطابق الفنان والعنوان والمدة")
		}
		var errs []string
		for _, v := range matches {
			path, e := s.fetchCandidate(ctx, t, v, p)
			if e == nil {
				return path, nil
			}
			errs = append(errs, e.Error())
		}
		return "", errors.New(strings.Join(errs, "؛ "))
	}
	switch c.Provider {
	case "spotify":
		return "", errors.New("Spotify مصدر بيانات؛ تتم محاولة المصادر البديلة تلقائيًا")
	case "soundcloud", "bandcamp", "ytmusic", "hearthis", "mixcloud", "jamendo", "audiomack", "reverbnation":
		return s.extractDownload(ctx, t, c, p)
	case "audius":
		if c.OriginalURL != "" {
			if path, e := s.transfer(ctx, t, c.OriginalURL); e == nil {
				return path, nil
			}
		}
		if c.AudioURL != "" {
			return s.transfer(ctx, t, c.AudioURL)
		}
		return s.extractDownload(ctx, t, c, p)
	case "tidal":
		address, e := s.tidalAudio(ctx, c)
		if e != nil {
			return "", e
		}
		if strings.HasPrefix(address, "dash:") {
			return s.fetchClearDASH(ctx, t, address)
		}
		return s.transfer(ctx, t, address)
	default:
		if c.AudioURL == "" {
			return "", errors.New("لا يوجد رابط صوت")
		}
		return s.transfer(ctx, t, c.AudioURL)
	}
}
func (s *Studio) CheckSources() map[string]string {
	r := map[string]string{}
	p := s.GetState().Preferences
	targets := map[string]string{"soundcloud": "https://soundcloud.com", "bandcamp": "https://bandcamp.com", "archive": "https://archive.org/metadata/testmp3testfile"}
	if p.TidalURL != "" {
		targets["tidal"] = p.TidalURL + "/"
	} else {
		r["tidal"] = "يحتاج إعداد خادم"
	}
	for id, site := range extraSourceSites {
		targets[id] = "https://" + site
	}
	var mu sync.Mutex
	ch := make(chan bool, len(targets))
	for name, address := range targets {
		go func(n, a string) {
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancel()
			req, _ := http.NewRequestWithContext(ctx, "GET", a, nil)
			res, e := s.client.Do(req)
			msg := "اتصال متاح؛ تنزيل الصوت يُختبر لكل أغنية"
			if e != nil {
				msg = "تعذر الاتصال"
			} else {
				res.Body.Close()
				if res.StatusCode >= 400 {
					msg = fmt.Sprintf("HTTP %d", res.StatusCode)
				}
			}
			mu.Lock()
			r[n] = msg
			mu.Unlock()
			ch <- true
		}(name, address)
	}
	for range targets {
		<-ch
	}
	r["direct"] = "جاهز لرابط صوت HTTP/HTTPS"
	return r
}

type flexBool bool

func (f *flexBool) UnmarshalJSON(b []byte) error {
	value := strings.ToLower(strings.Trim(string(b), `"`))
	*f = value == "true" || value == "1"
	return nil
}
func parseDuration(v string) float64 {
	parts := strings.Split(v, ":")
	var duration float64
	for _, part := range parts {
		value, e := strconv.ParseFloat(part, 64)
		if e != nil {
			return 0
		}
		duration = duration*60 + value
	}
	return duration
}

// Prefer the extractor's validated thumbnail. YouTube's thumbnail list also
// contains speculative maxres/WebP URLs that may return 404 for older videos.
func extractorArtwork(provider string, v *extractorInfo) string {
	if v.Thumbnail != "" {
		return v.Thumbnail
	}
	if provider == "ytmusic" && regexp.MustCompile(`^[a-zA-Z0-9_-]{11}$`).MatchString(v.ID) {
		return "https://i.ytimg.com/vi/" + v.ID + "/hqdefault.jpg"
	}
	artwork := ""
	for _, thumb := range v.Thumbnails {
		if thumb.URL != "" {
			artwork = thumb.URL
		}
	}
	return artwork
}
