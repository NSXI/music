package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testStudio(t *testing.T) *Studio {
	t.Helper()
	s := NewStudio(t.TempDir())
	s.state.Preferences.Lyrics = false
	tools, _ := filepath.Abs("tools")
	if _, e := os.Stat(tools); e != nil {
		tools, _ = filepath.Abs("../tools")
	}
	t.Setenv("PATH", tools+string(os.PathListSeparator)+os.Getenv("PATH"))
	return s
}
func testTask(s *Studio, id string) Task {
	v := Song{ID: id, Title: "Fixture recording", Artist: "Madar Tests", Album: "Fixture", Duration: 1, Provider: "direct"}
	task := Task{ID: key(id), Song: v, Status: "working", Log: []string{}}
	s.state.Jobs = append(s.state.Jobs, task)
	return task
}
func fixtureAudio(t *testing.T, s *Studio) []byte {
	t.Helper()
	path := filepath.Join(s.root, "fixture.flac")
	out, e := s.command(context.Background(), "ffmpeg", "-v", "error", "-nostdin", "-y", "-f", "lavfi", "-i", "sine=frequency=400:sample_rate=96000:duration=1", "-ac", "2", "-c:a", "flac", "-sample_fmt", "s32", path).CombinedOutput()
	if e != nil {
		t.Fatalf("fixture: %v %s", e, out)
	}
	b, _ := os.ReadFile(path)
	return b
}
func TestMatchingPreventsWrongVersion(t *testing.T) {
	a := Song{ID: "a", Title: "A song", Artist: "Artist", Duration: 200}
	b := a
	b.ID = "b"
	if !sameSong(a, b) {
		t.Fatal("matching recording rejected")
	}
	for _, edit := range []func(*Song){func(s *Song) { s.Title += " (Live)" }, func(s *Song) { s.Artist = "Cover artist" }, func(s *Song) { s.Duration = 240 }, func(s *Song) { s.Duration = 0 }} {
		c := b
		edit(&c)
		if sameSong(a, c) {
			t.Fatal("unverified/wrong recording accepted")
		}
	}
}
func TestQueueDedupAndRestart(t *testing.T) {
	s := testStudio(t)
	a := Song{ID: "s1", Title: "Song", Artist: "A", Duration: 10}
	b := a
	b.ID = "s2"
	s.known[a.ID] = a
	s.known[b.ID] = b
	n, e := s.Enqueue([]string{a.ID, b.ID, a.ID})
	if e != nil || n != 1 {
		t.Fatalf("dedup %d %v", n, e)
	}
	s.state.Jobs[0].Status = "working"
	s.save()
	r := NewStudio(s.root)
	if len(r.state.Jobs) != 1 || r.state.Jobs[0].Status != "queued" {
		t.Fatal("queue did not recover")
	}
	if e = r.Control("", "pause"); e != nil || r.state.Jobs[0].Status != "paused" {
		t.Fatal("pause")
	}
	r.Control("", "resume")
	if r.state.Jobs[0].Status != "queued" {
		t.Fatal("resume")
	}
}
func TestInterruptedTransferResumesWithValidator(t *testing.T) {
	s := testStudio(t)
	task := testTask(s, "resume")
	data := bytes.Repeat([]byte("audio-payload-"), 1000)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		w.Header().Set("ETag", `"version-1"`)
		if n == 1 {
			w.Header().Set("Content-Length", fmt.Sprint(len(data)))
			w.Write(data[:4096])
			return
		}
		if r.Header.Get("Range") != "bytes=4096-" || r.Header.Get("If-Range") != `"version-1"` {
			t.Errorf("invalid resume headers %v", r.Header)
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes 4096-%d/%d", len(data)-1, len(data)))
		w.Header().Set("Content-Length", fmt.Sprint(len(data)-4096))
		w.WriteHeader(206)
		w.Write(data[4096:])
	}))
	defer server.Close()
	path, e := s.transfer(context.Background(), task, server.URL)
	if e != nil {
		t.Fatal(e)
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(data, got) || calls.Load() != 2 {
		t.Fatal("resumed file differs")
	}
}
func TestTransferRejectsHTMLAndInvalidRanges(t *testing.T) {
	for _, mode := range []string{"html", "range", "etag"} {
		t.Run(mode, func(t *testing.T) {
			s := testStudio(t)
			task := testTask(s, mode)
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if mode == "html" {
					w.Header().Set("Content-Type", "text/html")
					io.WriteString(w, "<html>blocked</html>")
					return
				}
				w.Header().Set("Content-Range", "bytes 3-6/7")
				w.Header().Set("ETag", `"changed"`)
				w.WriteHeader(206)
				w.Write([]byte("abcd"))
			}))
			defer server.Close()
			os.MkdirAll(filepath.Join(s.root, "data", "partials"), 0755)
			os.WriteFile(filepath.Join(s.root, "data", "partials", task.ID+".part"), []byte("abc"), 0600)
			s.edit(task.ID, func(j *Task) { j.TransferURL = server.URL; j.ETag = `"old"` })
			if _, e := s.transfer(context.Background(), task, server.URL); e == nil {
				t.Fatal("invalid response accepted")
			}
		})
	}
}
func TestTransferRestartsWhenRepresentationChanges(t *testing.T) {
	s := testStudio(t)
	task := testTask(s, "changed")
	data := []byte("new-complete-recording")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "" {
			t.Error("different source must not resume")
		}
		w.Write(data)
	}))
	defer server.Close()
	os.MkdirAll(filepath.Join(s.root, "data", "partials"), 0755)
	os.WriteFile(filepath.Join(s.root, "data", "partials", task.ID+".part"), []byte("old-part"), 0600)
	s.edit(task.ID, func(j *Task) { j.TransferURL = "https://different.example/audio"; j.ETag = `"old"` })
	path, e := s.transfer(context.Background(), task, server.URL)
	if e != nil {
		t.Fatal(e)
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, data) {
		t.Fatal("old and new representations mixed")
	}
}
func TestRateLimitAndCancel(t *testing.T) {
	s := testStudio(t)
	task := testTask(s, "rate")
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(429)
			return
		}
		io.WriteString(w, "file")
	}))
	defer server.Close()
	if _, e := s.transfer(context.Background(), task, server.URL); e != nil || calls.Load() != 2 {
		t.Fatalf("retry %v %d", e, calls.Load())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e := s.waitRetry(ctx, task.ID, time.Hour); e == nil {
		t.Fatal("cancel ignored")
	}
}
func TestFullAudioValidationAndLosslessTruth(t *testing.T) {
	s := testStudio(t)
	b := fixtureAudio(t, s)
	path := filepath.Join(s.root, "fixture.flac")
	q, e := s.probe(context.Background(), path)
	if e != nil || !q.Lossless || q.Rate != 96000 || q.Bits != 24 {
		t.Fatalf("quality %+v %v", q, e)
	}
	if e = s.verify(context.Background(), path); e != nil {
		t.Fatal(e)
	}
	mp3 := filepath.Join(s.root, "lossy.flac")
	out, e := s.command(context.Background(), "ffmpeg", "-v", "error", "-nostdin", "-y", "-i", path, "-c:a", "libmp3lame", "-b:a", "128k", "-f", "mp3", mp3).CombinedOutput()
	if e != nil {
		t.Fatalf("%v %s", e, out)
	}
	q, e = s.probe(context.Background(), mp3)
	if e != nil || q.Lossless || q.Codec != "MP3" {
		t.Fatal("extension mistaken for lossless")
	}
	os.WriteFile(path, b[:100], 0600)
	if e = s.verify(context.Background(), path); e == nil {
		t.Fatal("truncated audio passed")
	}
}
func TestAutomaticAlternativeActualDownload(t *testing.T) {
	s := testStudio(t)
	payload := fixtureAudio(t, s)
	task := testTask(s, "fallback")
	task.Song.Provider = "unavailable"
	s.state.Jobs[0] = task
	var base string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/search/":
			fmt.Fprint(w, `{"data":{"tracks":{"items":[{"id":123,"title":"Fixture recording","duration":1,"artist":{"name":"Madar Tests"},"album":{"title":"Fixture"}}]}}}`)
		case "/track/":
			manifest, _ := json.Marshal(map[string]any{"encryptionType": "NONE", "urls": []string{base + "/audio"}})
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"manifest": base64.StdEncoding.EncodeToString(manifest)}})
		case "/audio":
			w.Header().Set("Content-Type", "audio/flac")
			w.Write(payload)
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	base = server.URL
	s.state.Preferences.TidalURL = server.URL
	if e := s.execute(context.Background(), task); e != nil {
		t.Fatal(e)
	}
	got, _ := s.task(task.ID)
	if got.Source != "tidal" || got.Path == "" || !got.Quality.Lossless {
		t.Fatalf("fallback did not deliver verified audio %+v", got)
	}
	if _, e := os.Stat(got.Path); e != nil {
		t.Fatal(e)
	}
	if len(got.Log) < 3 {
		t.Fatal("source attempts missing")
	}
}
func TestEncryptedManifestRejected(t *testing.T) {
	s := testStudio(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := base64.StdEncoding.EncodeToString([]byte(`{"encryptionType":"AES","urls":["https://example.com/protected"]}`))
		json.NewEncoder(w).Encode(map[string]string{"manifest": b})
	}))
	defer server.Close()
	s.state.Preferences.TidalURL = server.URL
	if _, e := s.tidalAudio(context.Background(), Song{Ref: "1"}); e == nil {
		t.Fatal("encrypted manifest accepted")
	}
}
func TestAtomicCommitNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source")
	dest := filepath.Join(dir, "dest")
	os.WriteFile(src, []byte("new"), 0600)
	os.WriteFile(dest, []byte("existing"), 0600)
	if e := copyAtomic(src, dest); e == nil {
		t.Fatal("existing destination replaced")
	}
	b, _ := os.ReadFile(dest)
	if string(b) != "existing" {
		t.Fatal("destination changed")
	}
}
func TestMediaHandlerRangeAndTraversal(t *testing.T) {
	s := testStudio(t)
	task := testTask(s, "media")
	path := filepath.Join(s.root, "audio.wav")
	os.WriteFile(path, []byte("0123456789"), 0600)
	s.edit(task.ID, func(j *Task) { j.Path = path })
	req := httptest.NewRequest("GET", "/media/"+task.ID, nil)
	req.Header.Set("Range", "bytes=2-5")
	w := httptest.NewRecorder()
	s.MediaHandler().ServeHTTP(w, req)
	if w.Code != 206 || w.Body.String() != "2345" {
		t.Fatal("media seeking failed")
	}
	w = httptest.NewRecorder()
	s.MediaHandler().ServeHTTP(w, httptest.NewRequest("GET", "/media/../../secret", nil))
	if w.Code != 404 {
		t.Fatal("arbitrary path served")
	}
}
func TestDemoAndLocalDuplicateImport(t *testing.T) {
	s := testStudio(t)
	n, e := s.Demo()
	if e != nil || n != 1 {
		t.Fatalf("demo %d %v", n, e)
	}
	n, e = s.Demo()
	if e != nil || n != 0 {
		t.Fatal("duplicate imported")
	}
	t0 := s.state.Jobs[0]
	if t0.Lyrics == "" || t0.Quality.Rate != 96000 {
		t.Fatal("demo missing local lyrics/quality")
	}
	p, e := s.PreparePlayback(t0.ID)
	if e != nil || !strings.HasPrefix(p.URL, "/media/") {
		t.Fatal("playback failed")
	}
}
func TestLiveSources(t *testing.T) {
	if os.Getenv("MADAR_LIVE") != "1" {
		t.Skip("set MADAR_LIVE=1 for real network and decoded-media verification")
	}
	s := testStudio(t)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	r, e := s.spotifyEmbed(ctx, "track", "4uLU6hMCjMI75M1A2tKUQC")
	if e != nil || len(r.Tracks) != 1 || r.Tracks[0].Artist == "" {
		t.Fatalf("Spotify metadata: %v %+v", e, r)
	}
	t.Logf("Spotify metadata confirmed: %s", r.Tracks[0].Title)
	task := testTask(s, "live-archive")
	task.Song.Title = "Internet Archive transfer test"
	task.Song.Duration = 0
	task.Song.AudioURL = "https://archive.org/download/testmp3testfile/mpthreetest.mp3"
	s.state.Jobs[0] = task
	s.state.Preferences.AutoMatch = false
	if e = s.execute(ctx, task); e != nil {
		t.Fatal(e)
	}
	saved, _ := s.task(task.ID)
	t.Logf("Actual transfer and full decoded validation: %s %d Hz %s", saved.Quality.Codec, saved.Quality.Rate, sizeForTest(saved.Path))
	for _, link := range []string{"https://youtube-dl.bandcamp.com/track/youtube-dl-test-song", "https://soundcloud.com/forss/flickermood"} {
		provider := "bandcamp"
		if strings.Contains(link, "soundcloud") {
			provider = "soundcloud"
		}
		r, e := s.extract(ctx, link, false, provider)
		if e != nil {
			t.Logf("%s unavailable in live check: %v", provider, e)
			continue
		}
		t.Logf("%s metadata %d tracks", provider, len(r.Tracks))
		if len(r.Tracks) > 0 {
			task := testTask(s, "live-"+provider)
			task.Song = r.Tracks[0]
			s.state.Jobs[len(s.state.Jobs)-1] = task
			path, e := s.fetchCandidate(ctx, task, task.Song, s.state.Preferences)
			if e != nil {
				t.Logf("%s transfer unavailable: %v", provider, e)
			} else if e = s.verify(ctx, path); e != nil {
				t.Errorf("%s decode: %v", provider, e)
			} else {
				q, _ := s.probe(ctx, path)
				t.Logf("%s actual audio validated %+v", provider, q)
			}
		}
	}
}
func sizeForTest(path string) string {
	st, e := os.Stat(path)
	if e != nil {
		return "missing"
	}
	return fmt.Sprint(st.Size()) + " bytes"
}
