package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

var artworkProviderNames = map[string]string{
	"artwork-itunes": "Apple iTunes", "artwork-deezer": "Deezer", "artwork-caa": "Cover Art Archive",
	"artwork-audiodb": "TheAudioDB", "artwork-fanart": "Fanart.tv",
}
var defaultArtworkProviders = []string{"artwork-itunes", "artwork-deezer", "artwork-caa", "artwork-audiodb", "artwork-fanart"}

type ArtworkCandidate struct {
	ID           string `json:"id"`
	Provider     string `json:"provider"`
	ProviderName string `json:"providerName"`
	URL          string `json:"url"`
	SourceURL    string `json:"sourceUrl"`
	PreviewURL   string `json:"previewUrl"`
	Title        string `json:"title"`
	Artist       string `json:"artist"`
	Album        string `json:"album"`
	Kind         string `json:"kind"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	Bytes        int64  `json:"bytes"`
	Format       string `json:"format"`
	Path         string `json:"path"`
	Sidecar      string `json:"sidecar,omitempty"`
	Cached       bool   `json:"cached"`
}
type ArtworkSearchResult struct {
	Candidates []ArtworkCandidate `json:"candidates"`
	Statuses   map[string]string  `json:"statuses"`
	Cached     bool               `json:"cached"`
	SelectedID string             `json:"selectedId"`
}
type artworkCache struct {
	Version int    `json:"version"`
	Song    Song   `json:"song"`
	SavedAt string `json:"savedAt"`
	ArtworkSearchResult
}

var artworkMu sync.Mutex
var artworkMusicBrainzMu sync.Mutex
var artworkMusicBrainzLast time.Time
var artworkToken = regexp.MustCompile(`^[0-9a-f]{20}-[0-9a-f]{20}$`)
var artworkImageName = regexp.MustCompile(`^[0-9a-f]{64}\.(jpg|png|gif)$`)

const artworkMaxBytes = 8 << 20
const artworkMaxPixels = 20_000_000

func artworkSongKey(song Song) string {
	return key(norm(song.Title) + "|" + norm(song.Artist) + "|" + norm(song.Album) + "|" + fmt.Sprintf("%.0f", song.Duration) + "|" + song.ISRC)
}
func artworkMatch(song Song, title, artist, album string, duration float64, albumOnly bool) bool {
	if norm(song.Artist) == "" || norm(artist) != norm(song.Artist) {
		return false
	}
	if albumOnly {
		return norm(song.Album) != "" && norm(album) == norm(song.Album)
	}
	if norm(song.Title) == "" || norm(title) != norm(song.Title) {
		return false
	}
	if song.Album != "" && album != "" && norm(song.Album) != norm(album) {
		return false
	}
	if song.Duration > 0 && duration > 0 {
		d := song.Duration - duration
		if d < 0 {
			d = -d
		}
		if d > 4 {
			return false
		}
	}
	return true
}
func artworkCandidate(song Song, provider, address, source, kind string) (ArtworkCandidate, bool) {
	if validWebURL(address) != nil {
		return ArtworkCandidate{}, false
	}
	if validWebURL(source) != nil {
		source = ""
	}
	return ArtworkCandidate{ID: key(provider + "|" + address), Provider: provider, ProviderName: artworkProviderNames[provider], URL: address, SourceURL: source, Title: song.Title, Artist: song.Artist, Album: song.Album, Kind: kind}, true
}
func artworkFirst(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func (s *Studio) artworkJSON(ctx context.Context, provider, base, endpoint string, params url.Values, target any, headers map[string]string) error {
	address := strings.TrimRight(s.metadataBase(provider, base), "/") + endpoint
	if len(params) > 0 {
		address += "?" + params.Encode()
	}
	if validWebURL(address) != nil {
		return errors.New("invalid artwork endpoint")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return errors.New("invalid artwork request")
	}
	req.Header.Set("User-Agent", "MadarMusic/1.0 (Windows desktop music library)")
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := s.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("artwork service request failed")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", res.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, (2<<20)+1))
	if err != nil {
		return errors.New("artwork metadata read failed")
	}
	if len(raw) > 2<<20 {
		return errors.New("artwork metadata is too large")
	}
	if err = json.Unmarshal(raw, target); err != nil {
		return errors.New("invalid artwork metadata response")
	}
	return nil
}

type artworkMBRelease struct {
	ID, Title    string
	ArtistCredit []struct {
		Name, JoinPhrase string
		Artist           struct{ ID, Name string }
	} `json:"artist-credit"`
	ReleaseGroup struct{ ID string } `json:"release-group"`
}

func artworkMBArtist(release artworkMBRelease) string {
	var b strings.Builder
	for _, a := range release.ArtistCredit {
		b.WriteString(artworkFirst(a.Name, a.Artist.Name))
		b.WriteString(a.JoinPhrase)
	}
	return b.String()
}
func artworkLucene(value string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value) + `"`
}
func (s *Studio) artworkRelease(ctx context.Context, song Song) (artworkMBRelease, error) {
	if strings.TrimSpace(song.Album) == "" {
		return artworkMBRelease{}, errors.New("album metadata required")
	}
	// MusicBrainz asks clients to stay at or below one request per second.
	if s.metadataBase("artwork-musicbrainz", "https://musicbrainz.org") == "https://musicbrainz.org" {
		artworkMusicBrainzMu.Lock()
		wait := time.Until(artworkMusicBrainzLast.Add(1100 * time.Millisecond))
		if wait > 0 {
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				artworkMusicBrainzMu.Unlock()
				return artworkMBRelease{}, ctx.Err()
			case <-timer.C:
			}
		}
		artworkMusicBrainzLast = time.Now()
		artworkMusicBrainzMu.Unlock()
	}
	var response struct {
		Releases []artworkMBRelease `json:"releases"`
	}
	err := s.artworkJSON(ctx, "artwork-musicbrainz", "https://musicbrainz.org", "/ws/2/release/", url.Values{"query": {"release:" + artworkLucene(song.Album) + " AND artist:" + artworkLucene(song.Artist)}, "fmt": {"json"}, "limit": {"5"}}, &response, nil)
	if err != nil {
		return artworkMBRelease{}, err
	}
	for _, release := range response.Releases {
		if artworkMatch(song, "", artworkMBArtist(release), release.Title, 0, true) {
			return release, nil
		}
	}
	return artworkMBRelease{}, errors.New("no matching album identity")
}
func (s *Studio) fetchArtworkProvider(ctx context.Context, song Song, provider string) ([]ArtworkCandidate, error) {
	items := []ArtworkCandidate{}
	add := func(address, source, kind string) {
		if c, ok := artworkCandidate(song, provider, address, source, kind); ok {
			items = append(items, c)
		}
	}
	switch provider {
	case "artwork-itunes":
		var response struct {
			Results []struct {
				Title      string  `json:"trackName"`
				Artist     string  `json:"artistName"`
				Album      string  `json:"collectionName"`
				Duration   float64 `json:"trackTimeMillis"`
				Artwork100 string  `json:"artworkUrl100"`
				Artwork600 string  `json:"artworkUrl600"`
				Link       string  `json:"trackViewUrl"`
			} `json:"results"`
		}
		if err := s.artworkJSON(ctx, provider, "https://itunes.apple.com", "/search", url.Values{"term": {song.Artist + " " + song.Title}, "entity": {"song"}, "media": {"music"}, "limit": {"10"}, "country": {"US"}}, &response, nil); err != nil {
			return nil, err
		}
		for _, row := range response.Results {
			if artworkMatch(song, row.Title, row.Artist, row.Album, row.Duration/1000, false) {
				add(artworkFirst(row.Artwork600, row.Artwork100), row.Link, "front")
			}
		}
	case "artwork-deezer":
		var response struct {
			Data []struct {
				Title    string
				Duration float64
				Link     string
				Artist   struct{ Name string }
				Album    struct {
					Title  string
					XL     string `json:"cover_xl"`
					Big    string `json:"cover_big"`
					Medium string `json:"cover_medium"`
					Cover  string
				}
			}
		}
		if err := s.artworkJSON(ctx, provider, "https://api.deezer.com", "/search", url.Values{"q": {song.Artist + " " + song.Title}, "limit": {"10"}}, &response, nil); err != nil {
			return nil, err
		}
		for _, row := range response.Data {
			if artworkMatch(song, row.Title, row.Artist.Name, row.Album.Title, row.Duration, false) {
				add(artworkFirst(row.Album.XL, row.Album.Big, row.Album.Medium, row.Album.Cover), row.Link, "front")
			}
		}
	case "artwork-caa":
		release, err := s.artworkRelease(ctx, song)
		if err != nil {
			return nil, err
		}
		var response struct {
			Images []struct {
				Image       string
				Front, Back bool
				Types       []string
			}
		}
		if err = s.artworkJSON(ctx, provider, "https://coverartarchive.org", "/release/"+url.PathEscape(release.ID), nil, &response, nil); err != nil {
			return nil, err
		}
		for _, img := range response.Images {
			if img.Front {
				add(img.Image, "https://musicbrainz.org/release/"+release.ID, "front")
			} else if img.Back {
				add(img.Image, "https://musicbrainz.org/release/"+release.ID, "back")
			}
		}
	case "artwork-audiodb":
		if strings.TrimSpace(song.Album) == "" {
			return nil, errors.New("album metadata required")
		}
		apiKey := s.metadataKey("audiodb")
		if apiKey == "" {
			apiKey = "123"
		}
		var response struct {
			Albums []struct {
				Album  string `json:"strAlbum"`
				Artist string `json:"strArtist"`
				Thumb  string `json:"strAlbumThumb"`
				HQ     string `json:"strAlbumThumbHQ"`
				Back   string `json:"strAlbumThumbBack"`
			} `json:"album"`
		}
		if err := s.artworkJSON(ctx, provider, "https://www.theaudiodb.com", "/api/v1/json/"+url.PathEscape(apiKey)+"/searchalbum.php", url.Values{"s": {song.Artist}, "a": {song.Album}}, &response, nil); err != nil {
			return nil, err
		}
		for _, row := range response.Albums {
			if artworkMatch(song, "", row.Artist, row.Album, 0, true) {
				add(artworkFirst(row.HQ, row.Thumb), "https://www.theaudiodb.com/", "front")
				add(row.Back, "https://www.theaudiodb.com/", "back")
			}
		}
	case "artwork-fanart":
		apiKey := s.metadataKey("fanart")
		if apiKey == "" {
			return nil, errors.New("API key required")
		}
		release, err := s.artworkRelease(ctx, song)
		if err != nil {
			return nil, err
		}
		if release.ReleaseGroup.ID == "" || len(release.ArtistCredit) != 1 || release.ArtistCredit[0].Artist.ID == "" {
			return nil, errors.New("verified MusicBrainz album and artist IDs required")
		}
		type fanartImage struct {
			URL           string
			Width, Height flexFloat
		}
		var response struct {
			Name   string
			Albums []struct {
				ID     string        `json:"release_group_id"`
				Covers []fanartImage `json:"albumcover"`
			}
			Backgrounds []fanartImage `json:"artistbackground"`
		}
		if err = s.artworkJSON(ctx, provider, "https://webservice.fanart.tv", "/v3.2/music/"+url.PathEscape(release.ArtistCredit[0].Artist.ID), nil, &response, map[string]string{"api-key": apiKey}); err != nil {
			return nil, err
		}
		if response.Name != "" && norm(response.Name) != norm(song.Artist) {
			return nil, errors.New("artist identity mismatch")
		}
		matched := false
		for _, album := range response.Albums {
			if album.ID == release.ReleaseGroup.ID {
				matched = true
				for _, img := range album.Covers {
					add(img.URL, "https://fanart.tv/", "front")
				}
			}
		}
		if matched {
			for _, img := range response.Backgrounds {
				add(img.URL, "https://fanart.tv/", "artist-background")
			}
		}
	default:
		return nil, errors.New("unknown artwork provider")
	}
	if len(items) == 0 {
		return nil, errors.New("no matching artwork")
	}
	if len(items) > 4 {
		items = items[:4]
	}
	return items, nil
}

func (s *Studio) artworkManifestPath(song Song) string {
	return filepath.Join(s.root, "data", "artwork", artworkSongKey(song)+".json")
}
func (s *Studio) artworkImageDir() string { return filepath.Join(s.root, "data", "artwork", "images") }
func (s *Studio) validArtworkPath(path string) bool {
	if !artworkImageName.MatchString(filepath.Base(path)) {
		return false
	}
	root, err := filepath.Abs(s.artworkImageDir())
	if err != nil {
		return false
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	relative, err := filepath.Rel(root, absolute)
	return err == nil && relative != "." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && relative != ".."
}

// cacheDirectArtwork saves only the cover supplied by the already selected audio
// source. It never starts the five-catalog search and is suitable for previews.
func (s *Studio) cacheDirectArtwork(ctx context.Context, song Song) (ArtworkCandidate, error) {
	if validWebURL(song.Artwork) != nil {
		return ArtworkCandidate{}, errors.New("source artwork URL unavailable")
	}
	artworkMu.Lock()
	defer artworkMu.Unlock()
	cache, err := s.readArtworkCache(song)
	if err != nil {
		cache = artworkCache{Version: 1, Song: song, ArtworkSearchResult: ArtworkSearchResult{Candidates: []ArtworkCandidate{}, Statuses: map[string]string{}}}
	}
	if cache.Statuses == nil {
		cache.Statuses = map[string]string{}
	}
	candidate, _ := artworkCandidate(song, "source-artwork", song.Artwork, song.URL, "front")
	candidate.ProviderName = "Track source"
	for _, cached := range cache.Candidates {
		if cached.ID == candidate.ID {
			return cached, nil
		}
	}
	candidate, err = s.cacheArtworkCandidate(ctx, song, candidate)
	if err != nil {
		return candidate, err
	}
	cache.Candidates = append(cache.Candidates, candidate)
	cache.Statuses["source-artwork"] = "source artwork saved locally"
	if err = s.writeArtworkCache(song, cache.ArtworkSearchResult); err != nil {
		return candidate, errors.New("source artwork fetched but cache save failed")
	}
	return candidate, nil
}
func (s *Studio) CacheSourceArtwork(id string) (ArtworkCandidate, error) {
	t, ok := s.task(id)
	if !ok {
		return ArtworkCandidate{}, errors.New("select a track first")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return s.cacheDirectArtwork(ctx, t.Song)
}
func readArtworkBytes(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, artworkMaxBytes+1))
	if err != nil || len(b) > artworkMaxBytes {
		return nil, errors.New("invalid artwork size")
	}
	return b, nil
}
func verifyArtworkBytes(raw []byte) (image.Config, string, error) {
	if len(raw) == 0 || len(raw) > artworkMaxBytes {
		return image.Config{}, "", errors.New("artwork exceeds size limit")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return config, "", errors.New("response is not a supported image")
	}
	if config.Width < 1 || config.Height < 1 || config.Width > 12000 || config.Height > 12000 || int64(config.Width)*int64(config.Height) > artworkMaxPixels {
		return config, "", errors.New("artwork dimensions exceed limit")
	}
	if format != "jpeg" && format != "png" && format != "gif" {
		return config, "", errors.New("unsupported artwork format")
	}
	if _, _, err = image.Decode(bytes.NewReader(raw)); err != nil {
		return config, "", errors.New("artwork image is incomplete")
	}
	return config, format, nil
}
func artworkAtomicNew(path string, raw []byte) error {
	if old, err := readArtworkBytes(path); err == nil {
		if bytes.Equal(old, raw) {
			return nil
		}
		return errors.New("existing artwork file differs")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".artwork-*.tmp")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(raw); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = moveNew(name, path); err != nil {
		if old, e := readArtworkBytes(path); e == nil && bytes.Equal(old, raw) {
			return nil
		}
		return err
	}
	return nil
}
func (s *Studio) cacheArtworkCandidate(ctx context.Context, song Song, c ArtworkCandidate) (ArtworkCandidate, error) {
	if c.Path != "" && s.validArtworkPath(c.Path) {
		if raw, err := readArtworkBytes(c.Path); err == nil {
			if config, format, e := verifyArtworkBytes(raw); e == nil {
				c.Cached = true
				c.Width = config.Width
				c.Height = config.Height
				c.Format = format
				c.Bytes = int64(len(raw))
				c.PreviewURL = "/artwork/" + artworkSongKey(song) + "-" + c.ID
				return c, nil
			}
		}
	}
	if validWebURL(c.URL) != nil {
		return c, errors.New("invalid artwork URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL, nil)
	if err != nil {
		return c, errors.New("invalid artwork request")
	}
	req.Header.Set("User-Agent", "MadarMusic/1.0")
	res, err := s.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return c, ctx.Err()
		}
		return c, errors.New("artwork image request failed")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return c, fmt.Errorf("image HTTP %d", res.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, artworkMaxBytes+1))
	if err != nil {
		return c, errors.New("artwork image read failed")
	}
	config, format, err := verifyArtworkBytes(raw)
	if err != nil {
		return c, err
	}
	digest := sha256.Sum256(raw)
	ext := format
	if ext == "jpeg" {
		ext = "jpg"
	}
	c.Path = filepath.Join(s.artworkImageDir(), hex.EncodeToString(digest[:])+"."+ext)
	if err = artworkAtomicNew(c.Path, raw); err != nil {
		return c, errors.New("artwork fetched but local save failed")
	}
	c.Format = format
	c.Width = config.Width
	c.Height = config.Height
	c.Bytes = int64(len(raw))
	c.Cached = true
	c.PreviewURL = "/artwork/" + artworkSongKey(song) + "-" + c.ID
	return c, nil
}
func (s *Studio) readArtworkCache(song Song) (artworkCache, error) {
	var cache artworkCache
	raw, err := os.ReadFile(s.artworkManifestPath(song))
	if err != nil {
		return cache, err
	}
	if len(raw) > 2<<20 {
		return cache, errors.New("artwork cache is too large")
	}
	if err = json.Unmarshal(raw, &cache); err != nil {
		return cache, err
	}
	if artworkSongKey(cache.Song) != artworkSongKey(song) {
		return cache, errors.New("artwork identity mismatch")
	}
	filtered := cache.Candidates[:0]
	for _, candidate := range cache.Candidates {
		if !s.validArtworkPath(candidate.Path) {
			continue
		}
		raw, e := readArtworkBytes(candidate.Path)
		if e != nil {
			continue
		}
		digest := sha256.Sum256(raw)
		if strings.TrimSuffix(filepath.Base(candidate.Path), filepath.Ext(candidate.Path)) != hex.EncodeToString(digest[:]) {
			continue
		}
		if _, _, e = verifyArtworkBytes(raw); e != nil {
			continue
		}
		candidate.Cached = true
		candidate.PreviewURL = "/artwork/" + artworkSongKey(song) + "-" + candidate.ID
		filtered = append(filtered, candidate)
	}
	cache.Candidates = filtered
	cache.Cached = true
	if len(filtered) == 0 {
		return cache, errors.New("no valid artwork in cache")
	}
	return cache, nil
}
func (s *Studio) writeArtworkCache(song Song, result ArtworkSearchResult) error {
	raw, err := json.MarshalIndent(artworkCache{Version: 1, Song: song, SavedAt: time.Now().UTC().Format(time.RFC3339), ArtworkSearchResult: result}, "", "  ")
	if err != nil {
		return err
	}
	return writeChanged(s.artworkManifestPath(song), raw)
}
func (s *Studio) searchArtwork(ctx context.Context, song Song, refresh bool) (ArtworkSearchResult, error) {
	artworkMu.Lock()
	defer artworkMu.Unlock()
	old, oldErr := s.readArtworkCache(song)
	if !refresh && oldErr == nil {
		return old.ArtworkSearchResult, nil
	}
	result := ArtworkSearchResult{Candidates: []ArtworkCandidate{}, Statuses: map[string]string{}, SelectedID: old.SelectedID}
	seen := map[string]bool{}
	oldCandidates := map[string]ArtworkCandidate{}
	for _, candidate := range old.Candidates {
		oldCandidates[candidate.ID] = candidate
	}
	for _, provider := range defaultArtworkProviders {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		run, cancel := context.WithTimeout(ctx, 10*time.Second)
		items, err := s.fetchArtworkProvider(run, song, provider)
		if err != nil {
			cancel()
			result.Statuses[provider] = err.Error()
			continue
		}
		saved := 0
		for _, candidate := range items {
			if seen[candidate.ID] {
				continue
			}
			seen[candidate.ID] = true
			if cached, ok := oldCandidates[candidate.ID]; ok {
				candidate = cached
			}
			candidate, err = s.cacheArtworkCandidate(run, song, candidate)
			if err == nil {
				result.Candidates = append(result.Candidates, candidate)
				saved++
			}
		}
		cancel()
		if saved > 0 {
			result.Statuses[provider] = fmt.Sprintf("%d images saved locally", saved)
		} else {
			result.Statuses[provider] = "matching images could not be verified or saved"
		}
	}
	if len(result.Candidates) == 0 {
		if oldErr == nil {
			old.Statuses = result.Statuses
			return old.ArtworkSearchResult, nil
		}
		return result, nil
	}
	// Keep a previous explicit selection available if a service no longer lists it.
	if old.SelectedID != "" && !seen[old.SelectedID] {
		if chosen, ok := oldCandidates[old.SelectedID]; ok {
			result.Candidates = append(result.Candidates, chosen)
		}
	}
	sort.SliceStable(result.Candidates, func(i, j int) bool {
		a, b := result.Candidates[i], result.Candidates[j]
		if (a.Kind == "front") != (b.Kind == "front") {
			return a.Kind == "front"
		}
		return int64(a.Width)*int64(a.Height) > int64(b.Width)*int64(b.Height)
	})
	if err := s.writeArtworkCache(song, result); err != nil {
		return result, errors.New("artwork fetched but cache index save failed")
	}
	return result, nil
}
func (s *Studio) SearchArtwork(id string, refresh bool) (ArtworkSearchResult, error) {
	t, ok := s.task(id)
	if !ok {
		return ArtworkSearchResult{}, errors.New("select a track first")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Second)
	defer cancel()
	return s.searchArtwork(ctx, t.Song, refresh)
}
func (s *Studio) ApplyArtwork(id, candidateID string) (ArtworkCandidate, error) {
	t, ok := s.task(id)
	if !ok {
		return ArtworkCandidate{}, errors.New("select a saved track first")
	}
	if t.Status == "preview" {
		return ArtworkCandidate{}, errors.New("save the audio before applying library artwork")
	}
	artworkMu.Lock()
	defer artworkMu.Unlock()
	cache, err := s.readArtworkCache(t.Song)
	if err != nil {
		return ArtworkCandidate{}, errors.New("fetch matching artwork first")
	}
	var selected *ArtworkCandidate
	for i := range cache.Candidates {
		if cache.Candidates[i].ID == candidateID {
			selected = &cache.Candidates[i]
			break
		}
	}
	if selected == nil {
		return ArtworkCandidate{}, errors.New("artwork selection is not in this track cache")
	}
	if t.Path != "" {
		raw, e := readArtworkBytes(selected.Path)
		if e == nil {
			ext := filepath.Ext(selected.Path)
			name := strings.TrimSuffix(t.Path, filepath.Ext(t.Path)) + ".madar-cover-" + strings.TrimSuffix(filepath.Base(selected.Path), ext)[:12] + ext
			if artworkAtomicNew(name, raw) == nil {
				selected.Sidecar = name
			}
		}
	}
	cache.SelectedID = selected.ID
	if err = s.writeArtworkCache(t.Song, cache.ArtworkSearchResult); err != nil {
		return ArtworkCandidate{}, errors.New("artwork selection save failed")
	}
	s.edit(t.ID, func(job *Task) { job.Cover = selected.Path })
	s.mu.Lock()
	s.save()
	storageErr := s.state.StorageError
	s.mu.Unlock()
	if storageErr != "" {
		return ArtworkCandidate{}, errors.New("library artwork save failed")
	}
	return *selected, nil
}
func (s *Studio) AutoArtwork(ctx context.Context, t Task) (string, error) {
	if t.Cover != "" {
		if raw, err := readArtworkBytes(t.Cover); err == nil {
			if _, _, err = verifyArtworkBytes(raw); err == nil {
				return t.Cover, nil
			}
		}
	}
	result, err := s.searchArtwork(ctx, t.Song, false)
	if err != nil {
		return "", err
	}
	for _, candidate := range result.Candidates {
		if candidate.Kind == "front" {
			selected, e := s.ApplyArtwork(t.ID, candidate.ID)
			if e == nil {
				return selected.Path, nil
			}
			return "", e
		}
	}
	return "", errors.New("no verified matching front cover available")
}
func (s *Studio) ServeArtwork(w http.ResponseWriter, r *http.Request, token string) {
	if !artworkToken.MatchString(token) {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(token, "-")
	raw, err := os.ReadFile(filepath.Join(s.root, "data", "artwork", parts[0]+".json"))
	if err != nil || len(raw) > 2<<20 {
		http.NotFound(w, r)
		return
	}
	var cache artworkCache
	if json.Unmarshal(raw, &cache) != nil || artworkSongKey(cache.Song) != parts[0] {
		http.NotFound(w, r)
		return
	}
	for _, candidate := range cache.Candidates {
		if candidate.ID == parts[1] && s.validArtworkPath(candidate.Path) {
			raw, readErr := readArtworkCachedBytes(candidate.Path)
			if readErr != nil {
				http.NotFound(w, r)
				return
			}
			digest := sha256.Sum256(raw)
			if strings.TrimSuffix(filepath.Base(candidate.Path), filepath.Ext(candidate.Path)) != hex.EncodeToString(digest[:]) {
				http.NotFound(w, r)
				return
			}
			if s.serveArtworkThumbnail(w, r, candidate.Path) {
				return
			}
			w.Header().Set("Cache-Control", "private, max-age=86400, immutable")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Content-Type", http.DetectContentType(raw))
			http.ServeContent(w, r, filepath.Base(candidate.Path), time.Time{}, bytes.NewReader(raw))
			return
		}
	}
	http.NotFound(w, r)
}
