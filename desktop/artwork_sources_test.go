package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type artworkRoundTripper func(*http.Request) (*http.Response, error)

func (f artworkRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func artworkPNG(t *testing.T) []byte {
	t.Helper()
	im := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			im.SetNRGBA(x, y, color.NRGBA{uint8(x * 3), uint8(y * 3), 120, 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, im); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func artworkFixture(t *testing.T) (*Studio, Task, *atomic.Int32, func()) {
	t.Helper()
	s := NewStudio(t.TempDir())
	var requests atomic.Int32
	raw := artworkPNG(t)
	var address string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		write := func(value any) { json.NewEncoder(w).Encode(value) }
		switch r.URL.Path {
		case "/itunes/search":
			write(map[string]any{"results": []any{map[string]any{"trackName": "Original Song", "artistName": "Madar Test", "collectionName": "Original Album", "trackTimeMillis": 12000, "artworkUrl100": address + "/images/itunes.png", "trackViewUrl": "https://music.apple.com/test"}}})
		case "/deezer/search":
			write(map[string]any{"data": []any{map[string]any{"title": "Original Song", "duration": 12, "artist": map[string]any{"name": "Madar Test"}, "album": map[string]any{"title": "Original Album", "cover_xl": address + "/images/deezer.png"}, "link": "https://www.deezer.com/track/test"}}})
		case "/mb/ws/2/release/":
			write(map[string]any{"releases": []any{map[string]any{"id": "release-fixture", "title": "Original Album", "artist-credit": []any{map[string]any{"name": "Madar Test", "artist": map[string]any{"id": "artist-fixture", "name": "Madar Test"}}}, "release-group": map[string]any{"id": "group-fixture"}}}})
		case "/caa/release/release-fixture":
			write(map[string]any{"images": []any{map[string]any{"front": true, "image": address + "/images/caa.png"}, map[string]any{"back": true, "image": address + "/images/caa-back.png"}}})
		case "/db/api/v1/json/123/searchalbum.php":
			write(map[string]any{"album": []any{map[string]any{"strArtist": "Madar Test", "strAlbum": "Original Album", "strAlbumThumbHQ": address + "/images/db.png"}}})
		case "/fan/v3.2/music/artist-fixture":
			if r.Header.Get("api-key") != "fixture-fanart-key" {
				t.Error("Fanart key must use a header")
			}
			if strings.Contains(r.URL.RawQuery, "fixture-fanart-key") {
				t.Error("key leaked to URL")
			}
			write(map[string]any{"name": "Madar Test", "albums": []any{map[string]any{"release_group_id": "group-fixture", "albumcover": []any{map[string]any{"url": address + "/images/fanart.png", "width": "1000", "height": "1000"}}}}, "artistbackground": []any{map[string]any{"url": address + "/images/poster.png"}}})
		default:
			if strings.HasPrefix(r.URL.Path, "/images/") {
				w.Header().Set("Content-Type", "image/png")
				w.Write(raw)
			} else {
				http.NotFound(w, r)
			}
		}
	}))
	address = server.URL
	s.sourceTestBases = map[string]string{"artwork-itunes": address + "/itunes", "artwork-deezer": address + "/deezer", "artwork-musicbrainz": address + "/mb", "artwork-caa": address + "/caa", "artwork-audiodb": address + "/db", "artwork-fanart": address + "/fan"}
	s.metadataKeys = map[string]string{"fanart": "fixture-fanart-key"}
	song := Song{ID: "artwork-original", Title: "Original Song", Artist: "Madar Test", Album: "Original Album", Duration: 12}
	path := filepath.Join(s.root, "Original Song.flac")
	os.WriteFile(path, []byte("original-audio-preserved"), 0600)
	task := Task{ID: "artwork-track", Song: song, Status: "done", Path: path}
	s.state.Jobs = []Task{task}
	return s, task, &requests, server.Close
}
func TestArtworkFiveProvidersAndOfflineCache(t *testing.T) {
	s, task, requests, closeServer := artworkFixture(t)
	defer closeServer()
	result, err := s.SearchArtwork(task.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Statuses) != 5 || len(result.Candidates) != 7 {
		t.Fatalf("expected five sources and seven verified images: %#v", result)
	}
	for _, candidate := range result.Candidates {
		if !candidate.Cached || candidate.Width != 64 || candidate.Height != 64 || candidate.Bytes <= 0 || candidate.Format != "png" || candidate.PreviewURL == "" {
			t.Fatalf("unverified dimensions or persistence: %#v", candidate)
		}
		if _, err = os.Stat(candidate.Path); err != nil {
			t.Fatal(err)
		}
	}
	files, _ := os.ReadDir(s.artworkImageDir())
	if len(files) != 1 {
		t.Fatalf("identical content not deduplicated: %d", len(files))
	}
	before := requests.Load()
	again, err := s.SearchArtwork(task.ID, false)
	if err != nil || !again.Cached || requests.Load() != before {
		t.Fatal("cached search used network", err)
	}
	chosen, err := s.ApplyArtwork(task.ID, result.Candidates[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if chosen.Sidecar == "" {
		t.Fatal("sidecar not saved")
	}
	if b, _ := os.ReadFile(task.Path); string(b) != "original-audio-preserved" {
		t.Fatal("audio changed")
	}
	firstInfo, _ := os.Stat(chosen.Sidecar)
	chosenAgain, err := s.ApplyArtwork(task.ID, chosen.ID)
	if err != nil || chosenAgain.Sidecar != chosen.Sidecar {
		t.Fatal("selection not stable", err)
	}
	secondInfo, _ := os.Stat(chosen.Sidecar)
	if firstInfo.ModTime() != secondInfo.ModTime() {
		t.Fatal("same sidecar rewritten")
	}
	restarted := NewStudio(s.root)
	restarted.client = &http.Client{Transport: artworkRoundTripper(func(*http.Request) (*http.Response, error) { return nil, errors.New("offline") })}
	offline, err := restarted.SearchArtwork(task.ID, false)
	if err != nil || !offline.Cached || offline.SelectedID != chosen.ID {
		t.Fatal("offline restart did not restore chosen artwork", err)
	}
	saved, _ := restarted.task(task.ID)
	if saved.Cover != chosen.Path {
		t.Fatal("library selection not persisted")
	}
	w := httptest.NewRecorder()
	restarted.ServeArtwork(w, httptest.NewRequest("GET", chosen.PreviewURL, nil), artworkSongKey(task.Song)+"-"+chosen.ID)
	if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), artworkPNG(t)) {
		t.Fatal("offline preview not served")
	}
	for _, token := range []string{"../library.json", strings.Repeat("a", 20) + "-" + strings.Repeat("b", 20)} {
		w = httptest.NewRecorder()
		restarted.ServeArtwork(w, httptest.NewRequest("GET", "/artwork/invalid", nil), token)
		if w.Code != 404 {
			t.Fatal("invalid artwork token accepted")
		}
	}
}
func TestArtworkProviderIdentityAndKeyBoundaries(t *testing.T) {
	s, task, _, closeServer := artworkFixture(t)
	defer closeServer()
	ctx := context.Background()
	for _, provider := range []string{"artwork-itunes", "artwork-deezer"} {
		song := task.Song
		song.Title += " (Live)"
		if _, err := s.fetchArtworkProvider(ctx, song, provider); err == nil {
			t.Fatal(provider, "accepted different recording")
		}
	}
	for _, provider := range []string{"artwork-caa", "artwork-audiodb", "artwork-fanart"} {
		song := task.Song
		song.Album = "Different Album"
		if _, err := s.fetchArtworkProvider(ctx, song, provider); err == nil {
			t.Fatal(provider, "accepted different album")
		}
	}
	s.metadataKeys = map[string]string{}
	if _, err := s.fetchArtworkProvider(ctx, task.Song, "artwork-fanart"); err == nil || !strings.Contains(err.Error(), "API key") {
		t.Fatal("missing Fanart key not explicit")
	}
	s.metadataKeys["audiodb"] = "fixture-private-key"
	s.client = &http.Client{Transport: artworkRoundTripper(func(r *http.Request) (*http.Response, error) { return nil, errors.New("failure at " + r.URL.String()) })}
	_, err := s.fetchArtworkProvider(ctx, task.Song, "artwork-audiodb")
	if err == nil || strings.Contains(err.Error(), "fixture-private-key") {
		t.Fatal("request error leaked key")
	}
}
func TestArtworkRejectsBadImagesAndPreservesOtherFiles(t *testing.T) {
	valid := artworkPNG(t)
	if _, _, err := verifyArtworkBytes([]byte("<!doctype html><h1>error</h1>")); err == nil {
		t.Fatal("HTML accepted")
	}
	if _, _, err := verifyArtworkBytes(valid[:len(valid)/2]); err == nil {
		t.Fatal("truncated PNG accepted")
	}
	oversize := append([]byte(nil), valid...)
	binary.BigEndian.PutUint32(oversize[16:20], 16000)
	binary.BigEndian.PutUint32(oversize[20:24], 16000)
	binary.BigEndian.PutUint32(oversize[29:33], crc32.ChecksumIEEE(oversize[12:29]))
	if _, _, err := verifyArtworkBytes(oversize); err == nil || !strings.Contains(err.Error(), "dimensions") {
		t.Fatal("oversized image accepted", err)
	}
	s, task, _, closeServer := artworkFixture(t)
	defer closeServer()
	result, err := s.SearchArtwork(task.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	candidate := result.Candidates[0]
	originalCover := strings.TrimSuffix(task.Path, ".flac") + ".png"
	os.WriteFile(originalCover, []byte("unrelated-original-cover"), 0600)
	selected, err := s.ApplyArtwork(task.ID, candidate.ID)
	if err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(originalCover); string(raw) != "unrelated-original-cover" {
		t.Fatal("unrelated sidecar overwritten")
	}
	path, err := s.AutoArtwork(context.Background(), Task{ID: task.ID, Song: task.Song, Cover: selected.Path, Path: task.Path})
	if err != nil || path != selected.Path {
		t.Fatal("existing cover not preserved", err)
	}
	if _, err = s.ApplyArtwork(task.ID, "not-a-result"); err == nil {
		t.Fatal("arbitrary selection accepted")
	}
}

func TestArtworkDirectSourcePreviewCachesWithoutCatalogSearch(t *testing.T) {
	s, task, requests, closeServer := artworkFixture(t)
	defer closeServer()
	task.Song.Artwork = s.sourceTestBases["artwork-itunes"][:len(s.sourceTestBases["artwork-itunes"])-len("/itunes")] + "/images/direct.png"
	before := requests.Load()
	candidate, err := s.cacheDirectArtwork(context.Background(), task.Song)
	if err != nil {
		t.Fatal(err)
	}
	if requests.Load() != before+1 || candidate.PreviewURL == "" || candidate.Provider != "source-artwork" {
		t.Fatal("direct cover triggered catalog search or was not cached")
	}
	before = requests.Load()
	again, err := s.cacheDirectArtwork(context.Background(), task.Song)
	if err != nil || again.Path != candidate.Path || requests.Load() != before {
		t.Fatal("direct cached cover used network", err)
	}
	if err = os.WriteFile(candidate.Path, []byte("corrupted image"), 0600); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	s.ServeArtwork(w, httptest.NewRequest("GET", candidate.PreviewURL, nil), artworkSongKey(task.Song)+"-"+candidate.ID)
	if w.Code != 404 {
		t.Fatal("corrupted image served")
	}
}

func TestArtworkPublicMetadataLive(t *testing.T) {
	if os.Getenv("MADAR_ARTWORK_LIVE") != "1" {
		t.Skip("MADAR_ARTWORK_LIVE=1 enables public metadata/image probes with a generic recording")
	}
	s := NewStudio(t.TempDir())
	song := Song{ID: "public-artwork-probe", Title: "Yellow", Artist: "Coldplay", Album: "Parachutes"}
	for _, provider := range defaultArtworkProviders {
		t.Run(provider, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 18*time.Second)
			defer cancel()
			items, err := s.fetchArtworkProvider(ctx, song, provider)
			if err != nil {
				t.Skip("source unavailable in this probe: ", err)
			}
			candidate, err := s.cacheArtworkCandidate(ctx, song, items[0])
			if err != nil {
				t.Skip("metadata matched but image unavailable in this probe: ", err)
			}
			if !candidate.Cached || candidate.Path == "" || candidate.Width < 1 || candidate.Height < 1 {
				t.Fatal("image not verified and persisted")
			}
			t.Logf("matched metadata and cached %s %dx%d (%d bytes)", candidate.Format, candidate.Width, candidate.Height, candidate.Bytes)
		})
	}
}
