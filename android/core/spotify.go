package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	wr "nawa.android/core/desktop"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type spotifyTrack struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Duration int64  `json:"duration_ms"`
	IsLocal  bool   `json:"is_local"`
	Type     string `json:"type"`
	Artists  []struct {
		Name string `json:"name"`
	} `json:"artists"`
	Album struct {
		Name   string `json:"name"`
		Images []struct {
			URL string `json:"url"`
		} `json:"images"`
	} `json:"album"`
	ExternalIDs struct {
		ISRC string `json:"isrc"`
	} `json:"external_ids"`
}

func spotifySong(t spotifyTrack) Song {
	a := []string{}
	for _, v := range t.Artists {
		a = append(a, v.Name)
	}
	art := ""
	if len(t.Album.Images) > 0 {
		art = t.Album.Images[0].URL
	}
	return Song{ID: "spotify-" + t.ID, Title: t.Name, Artist: strings.Join(a, ", "), Album: t.Album.Name, Duration: float64(t.Duration) / 1000, Artwork: art, Provider: "spotify", URL: "https://open.spotify.com/track/" + t.ID, Ref: t.ID, ISRC: t.ExternalIDs.ISRC}
}
func (s *Studio) spotifyJSON(ctx context.Context, path string, target any) error {
	if e := s.ensureSpotifyToken(ctx); e != nil {
		return e
	}
	s.mu.Lock()
	token := s.spotifyToken
	s.mu.Unlock()
	if token == "" {
		return errors.New("اربط حساب Spotify من الإعدادات للبحث واستيراد القوائم الكاملة")
	}
	if strings.HasPrefix(path, "https://api.spotify.com/v1/") {
		path = strings.TrimPrefix(path, "https://api.spotify.com/v1")
	}
	if !strings.HasPrefix(path, "/") || strings.Contains(path, "://") {
		return errors.New("رابط صفحة Spotify غير صالح")
	}
	for attempt := 0; attempt < 3; attempt++ {
		req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.spotify.com/v1"+path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		r, e := s.client.Do(req)
		if e != nil {
			return e
		}
		if r.StatusCode == 429 && attempt < 2 {
			d := retryDelay(r)
			r.Body.Close()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(d):
			}
			continue
		}
		defer r.Body.Close()
		if r.StatusCode != 200 {
			return fmt.Errorf("Spotify HTTP %d؛ تحقق من صلاحيات التطبيق والحساب", r.StatusCode)
		}
		return json.NewDecoder(io.LimitReader(r.Body, 12<<20)).Decode(target)
	}
	return errors.New("Spotify يحد الطلبات مؤقتًا")
}
func (s *Studio) spotifySearch(ctx context.Context, q string) (Result, error) {
	var d struct {
		Tracks struct {
			Items []spotifyTrack `json:"items"`
		} `json:"tracks"`
	}
	if e := s.spotifyJSON(ctx, "/search?type=track&limit=10&q="+url.QueryEscape(q), &d); e != nil {
		return Result{}, e
	}
	r := Result{Title: q, Tracks: []Song{}, Complete: false, Notice: "نتائج بيانات Spotify؛ يبحث التنزيل عن تسجيل مطابق لدى المصادر البديلة."}
	for _, t := range d.Tracks.Items {
		r.Tracks = append(r.Tracks, spotifySong(t))
	}
	r.Total = len(r.Tracks)
	return r, nil
}
func (s *Studio) spotify(ctx context.Context, u *url.URL) (Result, error) {
	m := regexp.MustCompile(`/(track|playlist|album)/([A-Za-z0-9]{22})`).FindStringSubmatch(u.Path)
	if len(m) != 3 {
		return Result{}, errors.New("ألصق رابط أغنية أو ألبوم أو قائمة Spotify")
	}
	s.mu.Lock()
	linked := s.spotifyToken != ""
	s.mu.Unlock()
	if !linked {
		return s.spotifyEmbed(ctx, m[1], m[2])
	}
	kind, id := m[1], m[2]
	r := Result{Tracks: []Song{}, Complete: true}
	if kind == "track" {
		var t spotifyTrack
		e := s.spotifyJSON(ctx, "/tracks/"+id, &t)
		if e != nil {
			return r, e
		}
		r.Title = t.Name
		r.Tracks = append(r.Tracks, spotifySong(t))
		r.Total = 1
		return r, nil
	}
	var meta struct {
		Name   string `json:"name"`
		Images []struct {
			URL string `json:"url"`
		} `json:"images"`
	}
	if e := s.spotifyJSON(ctx, "/"+kind+"s/"+id, &meta); e != nil {
		return r, e
	}
	r.Title = meta.Name
	path := "/" + kind + "s/" + id + "/tracks?limit=50"
	if kind == "playlist" {
		path = "/playlists/" + id + "/items?limit=50"
	}
	seenPages := map[string]bool{}
	seenTracks := map[string]bool{}
	skipped := 0
	for path != "" {
		if seenPages[path] {
			r.Complete = false
			break
		}
		seenPages[path] = true
		var page struct {
			Items []json.RawMessage `json:"items"`
			Next  string            `json:"next"`
			Total int               `json:"total"`
		}
		e := s.spotifyJSON(ctx, path, &page)
		if e != nil {
			if len(r.Tracks) == 0 {
				return r, e
			}
			r.Complete = false
			r.Notice = "توقف جلب الصفحات: " + e.Error()
			break
		}
		r.Total = page.Total
		for _, raw := range page.Items {
			var t spotifyTrack
			if kind == "playlist" {
				var row struct {
					Track *spotifyTrack `json:"track"`
					Item  *spotifyTrack `json:"item"`
				}
				json.Unmarshal(raw, &row)
				if row.Item != nil {
					t = *row.Item
				} else if row.Track != nil {
					t = *row.Track
				}
			} else {
				json.Unmarshal(raw, &t)
				t.Album.Name = meta.Name
				if len(meta.Images) > 0 {
					t.Album.Images = append(t.Album.Images, struct {
						URL string `json:"url"`
					}{meta.Images[0].URL})
				}
			}
			if t.ID == "" || t.IsLocal || (t.Type != "" && t.Type != "track") {
				skipped++
				continue
			}
			if seenTracks[t.ID] {
				skipped++
				continue
			}
			seenTracks[t.ID] = true
			r.Tracks = append(r.Tracks, spotifySong(t))
		}
		path = page.Next
	}
	r.Notice += fmt.Sprintf(" جُلب %d من أصل %d عنصر. أزيل %d مكرر أو عنصر غير موسيقي. الصوت من المصادر البديلة.", len(r.Tracks), r.Total, skipped)
	return r, nil
}
func (s *Studio) spotifyEmbed(ctx context.Context, kind, id string) (Result, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://open.spotify.com/embed/"+kind+"/"+id, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	res, e := s.client.Do(req)
	if e != nil {
		return Result{}, e
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return Result{}, fmt.Errorf("Spotify HTTP %d", res.StatusCode)
	}
	b, e := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	if e != nil {
		return Result{}, e
	}
	m := regexp.MustCompile(`(?s)<script[^>]*id="__NEXT_DATA__"[^>]*>(.*?)</script>`).FindSubmatch(b)
	if len(m) != 2 {
		return Result{}, errors.New("Spotify لم يعرض البيانات العامة. اربط حسابك من الإعدادات أو أعد المحاولة")
	}
	var d struct {
		Props struct {
			Page struct {
				State struct {
					Data struct {
						Entity struct {
							ID       string  `json:"id"`
							Type     string  `json:"type"`
							Name     string  `json:"name"`
							Title    string  `json:"title"`
							Duration float64 `json:"duration"`
							Artists  []struct {
								Name string `json:"name"`
							} `json:"artists"`
							Visual struct {
								Image []struct {
									URL  string `json:"url"`
									Size int    `json:"maxWidth"`
								} `json:"image"`
							} `json:"visualIdentity"`
							Tracks []struct {
								URI      string  `json:"uri"`
								Title    string  `json:"title"`
								Subtitle string  `json:"subtitle"`
								Duration float64 `json:"duration"`
								Type     string  `json:"entityType"`
							} `json:"trackList"`
						} `json:"entity"`
					} `json:"data"`
				} `json:"state"`
			} `json:"pageProps"`
		} `json:"props"`
	}
	if e = json.Unmarshal(m[1], &d); e != nil {
		return Result{}, e
	}
	entity := d.Props.Page.State.Data.Entity
	if entity.ID != id {
		return Result{}, errors.New("المحتوى غير متاح للعرض العام")
	}
	title := entity.Title
	if title == "" {
		title = entity.Name
	}
	r := Result{Title: title, Tracks: []Song{}, Complete: kind == "track", Notice: "استيراد العرض العام من Spotify. القوائم قد تكون جزئية؛ اربط حسابك من الإعدادات لجلب كل الصفحات المتاحة."}
	if kind == "track" {
		a := []string{}
		for _, v := range entity.Artists {
			a = append(a, v.Name)
		}
		art := ""
		largest := 0
		for _, im := range entity.Visual.Image {
			if im.Size > largest {
				largest = im.Size
				art = im.URL
			}
		}
		r.Tracks = append(r.Tracks, Song{ID: "spotify-" + id, Title: title, Artist: strings.Join(a, ", "), Duration: entity.Duration / 1000, Artwork: art, Provider: "spotify", URL: "https://open.spotify.com/track/" + id, Ref: id})
	} else {
		for _, v := range entity.Tracks {
			if !strings.HasPrefix(v.URI, "spotify:track:") {
				continue
			}
			ref := strings.TrimPrefix(v.URI, "spotify:track:")
			r.Tracks = append(r.Tracks, Song{ID: "spotify-" + ref, Title: v.Title, Artist: v.Subtitle, Duration: v.Duration / 1000, Provider: "spotify", URL: "https://open.spotify.com/track/" + ref, Ref: ref})
		}
	}
	r.Total = len(r.Tracks)
	return r, nil
}
func (s *Studio) ConnectSpotify(clientID string) error {
	s.oauthMu.Lock()
	defer s.oauthMu.Unlock()
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return errors.New("أضف Client ID الخاص بتطبيق Spotify أولًا")
	}
	listener, e := net.Listen("tcp", "127.0.0.1:43783")
	if e != nil {
		return errors.New("منفذ ربط Spotify مشغول؛ أعد المحاولة")
	}
	random := make([]byte, 48)
	rand.Read(random)
	verifier := base64.RawURLEncoding.EncodeToString(random)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	state := key(verifier + time.Now().String())
	redirect := "http://127.0.0.1:43783/callback"
	done := make(chan error, 1)
	mux := http.NewServeMux()
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	defer server.Close()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			http.Error(w, "Invalid OAuth state", 400)
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			select {
			case done <- errors.New("تم إلغاء الربط"):
			default:
			}
			fmt.Fprint(w, "You can return to Madar Music.")
			return
		}
		values := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {redirect}, "client_id": {clientID}, "code_verifier": {verifier}}
		req, _ := http.NewRequest("POST", "https://accounts.spotify.com/api/token", strings.NewReader(values.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		res, err := s.client.Do(req)
		if err == nil {
			defer res.Body.Close()
			if res.StatusCode != 200 {
				err = fmt.Errorf("Spotify token HTTP %d", res.StatusCode)
			} else {
				var token struct {
					Access  string `json:"access_token"`
					Refresh string `json:"refresh_token"`
					Expires int    `json:"expires_in"`
				}
				err = json.NewDecoder(res.Body).Decode(&token)
				if err == nil && token.Access == "" {
					err = errors.New("لم يصل تصريح Spotify")
				}
				if err == nil {
					s.mu.Lock()
					s.spotifyToken = token.Access
					s.spotifyRefresh = token.Refresh
					s.spotifyExpiry = time.Now().Add(time.Duration(token.Expires) * time.Second)
					s.state.Preferences.SpotifyClientID = clientID
					s.save()
					s.mu.Unlock()
					if er := s.saveAuth(); er != nil {
						err = fmt.Errorf("تم الربط ولكن تعذر حفظ التصريح الآمن: %w", er)
					}
				}
			}
		}
		select {
		case done <- err:
		default:
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, "<html dir=rtl><h2>تم استلام الرد. ارجع إلى مدار Music.</h2></html>")
	})
	go server.Serve(listener)
	params := url.Values{"client_id": {clientID}, "response_type": {"code"}, "redirect_uri": {redirect}, "code_challenge_method": {"S256"}, "code_challenge": {challenge}, "state": {state}, "scope": {"playlist-read-private playlist-read-collaborative user-library-read"}}
	wr.BrowserOpenURL(s.ctx, "https://accounts.spotify.com/authorize?"+params.Encode())
	select {
	case e := <-done:
		return e
	case <-time.After(3 * time.Minute):
		return errors.New("انتهت مهلة الربط؛ أعد المحاولة")
	}
}
