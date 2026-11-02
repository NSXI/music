package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAdditionalSourceContracts(t *testing.T) {
	for _, provider := range []string{"audius", "hearthis", "mixcloud", "commons"} {
		t.Run(provider, func(t *testing.T) {
			s := testStudio(t)
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch provider {
				case "audius":
					json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"id": "ok", "title": "Test", "duration": 42, "permalink": "/artist/test", "is_streamable": true, "is_stream_gated": false, "is_downloadable": true, "is_download_gated": false, "artwork": map[string]string{"1000x1000": server.URL + "/cover.jpg"}, "user": map[string]string{"name": "Artist"}}, map[string]any{"id": "gated", "title": "Protected", "is_streamable": true, "is_stream_gated": true}}})
				case "hearthis":
					io.WriteString(w, `[{"id":"1","title":"Test","duration":"42","permalink_url":"https://hearthis.at/artist/test/","stream_url":"https://audio.example/file.mp3","private":"0","artwork_url":"https://art.example/cover.jpg","user":{"username":"Artist"}}]`)
				case "mixcloud":
					io.WriteString(w, `{"data":[{"key":"/artist/show/","url":"https://www.mixcloud.com/artist/show/","name":"Test","audio_length":42,"pictures":{"large":"https://art.example/cover.jpg"},"user":{"name":"Artist"}}]}`)
				case "commons":
					io.WriteString(w, `{"query":{"pages":{"1":{"title":"File:Test.ogg","imageinfo":[{"url":"https://audio.example/test.ogg","descriptionurl":"https://commons.wikimedia.org/wiki/File:Test.ogg","mime":"audio/ogg","duration":42,"size":1000,"extmetadata":{"ObjectName":{"value":"Test"},"Artist":{"value":"<b>Artist</b>"}}}]}}}}`)
				}
			}))
			defer server.Close()
			s.sourceTestBases = map[string]string{provider: server.URL}
			r, e := s.providerSearch(context.Background(), provider, "test")
			if e != nil || len(r.Tracks) != 1 || r.Tracks[0].Provider != provider || r.Tracks[0].Artist != "Artist" {
				t.Fatalf("%+v %v", r, e)
			}
			if provider == "audius" && (r.Tracks[0].OriginalURL == "" || r.Tracks[0].Artwork == "") {
				t.Fatal("missing permitted original or cover")
			}
		})
	}
	if len(extraSources) != 8 {
		t.Fatal(extraSources)
	}
	for _, item := range []struct{ host, provider string }{{"music.youtube.com", "ytmusic"}, {"youtu.be", "ytmusic"}, {"audiomack.com", "audiomack"}, {"www.reverbnation.com", "reverbnation"}} {
		if sourceForHost(item.host) != item.provider {
			t.Fatal(item)
		}
	}
	if indexedSourceLink("jamendo", "https://jamendo.com.attacker.invalid/track/12") {
		t.Fatal("wrong indexed host accepted")
	}
}
func TestPreviewRangeAndNoLibraryDownload(t *testing.T) {
	data := strings.Repeat("audio-data-", 100)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		http.ServeContent(w, r, "test.mp3", time.Time{}, strings.NewReader(data))
	}))
	defer upstream.Close()
	s := testStudio(t)
	song := Song{ID: "preview-only", Title: "Test preview", Artist: "Madar Tests", Provider: "direct", URL: upstream.URL, AudioURL: upstream.URL, Artwork: upstream.URL + "/cover.jpg", Duration: 30}
	s.known[song.ID] = song
	p, e := s.PreparePreview(song.ID, false)
	if e != nil {
		t.Fatal(e)
	}
	if p.Song.Artwork == "" || len(s.GetState().Jobs) != 0 {
		t.Fatal("preview lost cover or created download")
	}
	req := httptest.NewRequest("GET", p.URL, nil)
	req.Header.Set("Range", "bytes=10-19")
	w := httptest.NewRecorder()
	s.MediaHandler().ServeHTTP(w, req)
	if w.Code != 206 || w.Body.String() != data[10:20] || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Code, w.Body.String(), w.Header())
	}
	head := httptest.NewRecorder()
	s.MediaHandler().ServeHTTP(head, httptest.NewRequest("HEAD", p.URL, nil))
	if head.Code != 200 || head.Body.Len() != 0 {
		t.Fatal("HEAD streamed a body")
	}
	if _, e = os.Stat(filepath.Join(s.root, "Downloads")); !os.IsNotExist(e) {
		t.Fatal("preview created download files")
	}
	// A stream can cache lyrics and translations without a saved audio file.
	s.writeLyricCache(LyricCache{Song: song, Record: LyricsRecord{Title: song.Title, Artist: song.Artist, Plain: "First original line\nSecond original line", Provider: "local"}})
	view, e := s.GetLyrics(song.ID, false)
	if e != nil || view.Text == "" {
		t.Fatal(view, e)
	}
	rows, _ := s.translationOriginal(Task{ID: song.ID, Song: song})
	if e = s.SaveTranslation(song.ID, "ar", "en", translationHash(rows), []string{"السطر الأول", "السطر الثاني"}); e != nil {
		t.Fatal(e)
	}
	if len(s.GetState().Jobs) != 0 || len(s.GetState().AlignmentQueue) != 0 {
		t.Fatal("stream polluted library or alignment queue")
	}
}
func TestPreviewRejectsHTMLAndExpiredSession(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, "<html>login</html>")
	}))
	defer upstream.Close()
	s := testStudio(t)
	song := Song{ID: "html", Provider: "direct", URL: upstream.URL, AudioURL: upstream.URL}
	s.known[song.ID] = song
	p, e := s.PreparePreview(song.ID, false)
	if e != nil {
		t.Fatal(e)
	}
	w := httptest.NewRecorder()
	s.MediaHandler().ServeHTTP(w, httptest.NewRequest("GET", p.URL, nil))
	if w.Code != 502 {
		t.Fatal(w.Code)
	}
	w = httptest.NewRecorder()
	s.MediaHandler().ServeHTTP(w, httptest.NewRequest("GET", "/stream/missing", nil))
	if w.Code != 410 {
		t.Fatal(w.Code)
	}
}
func TestGenresPersistFromFileAndManualEdit(t *testing.T) {
	s := testStudio(t)
	file := filepath.Join(s.root, "tagged.flac")
	out, e := s.command(context.Background(), "ffmpeg", "-v", "error", "-f", "lavfi", "-i", "sine=duration=1", "-metadata", "genre=Rock; Alternative Metal", "-c:a", "flac", file).CombinedOutput()
	if e != nil {
		t.Fatal(e, string(out))
	}
	if n, e := s.importPaths(context.Background(), []string{file}); e != nil || n != 1 {
		t.Fatal(n, e)
	}
	job := s.GetState().Jobs[0]
	if len(job.Song.Genres) != 2 || !job.GenreChecked {
		t.Fatal(job)
	}
	if e = s.SetGenres(job.ID, []string{"Jazz", "jazz", "موسيقى عربية"}); e != nil {
		t.Fatal(e)
	}
	restarted := NewStudio(s.root)
	if len(restarted.GetState().Jobs[0].Song.Genres) != 2 {
		t.Fatal("manual genres were not persisted")
	}
}
func TestInstalledFontsAvailable(t *testing.T) {
	fonts := testStudio(t).GetFonts()
	if len(fonts) < 3 {
		t.Fatal(fonts)
	}
	t.Logf("Enumerated %d font families", len(fonts))
}

func TestDecodedPreviewStreamsActualFloatAudio(t *testing.T) {
	s := testStudio(t)
	data := fixtureAudio(t, s)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/flac")
		http.ServeContent(w, r, "test.flac", time.Time{}, strings.NewReader(string(data)))
	}))
	defer upstream.Close()
	song := Song{ID: "decoded-preview", Provider: "direct", AudioURL: upstream.URL, URL: upstream.URL, Title: "Original local tone", Duration: 1}
	s.known[song.ID] = song
	p, e := s.PreparePreview(song.ID, true)
	if e != nil {
		t.Fatal(e)
	}
	w := httptest.NewRecorder()
	s.MediaHandler().ServeHTTP(w, httptest.NewRequest("GET", p.URL, nil))
	if w.Code != 200 || !strings.HasPrefix(w.Body.String(), "RIFF") {
		t.Fatal(w.Code, w.Body.String())
	}
	path := filepath.Join(s.root, "preview-float.wav")
	os.WriteFile(path, w.Body.Bytes(), 0600)
	q, e := s.probe(context.Background(), path)
	if e != nil || q.Codec != "PCM_F32LE" || q.Rate != 96000 || q.Channels != 2 {
		t.Fatal(q, e)
	}
	if len(s.GetState().Jobs) != 0 {
		t.Fatal("stream was saved to library")
	}
}

func TestLoopbackPreviewOriginAndShutdown(t *testing.T) {
	s := testStudio(t)
	s.startPreviewServer()
	if s.previewServer == nil {
		t.Fatal(s.previewServerError)
	}
	defer s.previewServer.Close()
	request, _ := http.NewRequest("GET", s.previewAddress+"/stream/missing", nil)
	request.Header.Set("Origin", "http://wails.localhost")
	response, e := http.DefaultClient.Do(request)
	if e != nil {
		t.Fatal(e)
	}
	response.Body.Close()
	if response.StatusCode != 410 || response.Header.Get("Access-Control-Allow-Origin") != "http://wails.localhost" {
		t.Fatal(response.Status, response.Header)
	}
	request, _ = http.NewRequest("GET", s.previewAddress+"/stream/missing", nil)
	request.Header.Set("Origin", "https://unrelated.example")
	response, e = http.DefaultClient.Do(request)
	if e != nil {
		t.Fatal(e)
	}
	response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatal(response.Status)
	}
	request, _ = http.NewRequest("OPTIONS", s.previewAddress+"/stream/missing", nil)
	request.Header.Set("Origin", "http://wails.localhost")
	response, e = http.DefaultClient.Do(request)
	if e != nil {
		t.Fatal(e)
	}
	response.Body.Close()
	if response.StatusCode != 204 {
		t.Fatal(response.Status)
	}
}
