package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const fixtureLRC = "[00:00.10]First test line\n[00:00.60]Second test line\n"

func lyricTask(t *testing.T, s *Studio) Task {
	t.Helper()
	task := testTask(s, "lyrics")
	task.Path = filepath.Join(s.root, "song.flac")
	s.state.Jobs[0] = task
	return task
}
func TestFiveLyricsProviderContracts(t *testing.T) {
	for _, provider := range []string{"lrclib", "qq", "netease", "kugou", "musixmatch"} {
		t.Run(provider, func(t *testing.T) {
			s := testStudio(t)
			task := lyricTask(t, s)
			s.musixmatchKey = "fixture-key"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch provider {
				case "lrclib":
					json.NewEncoder(w).Encode(LyricsRecord{Title: task.Song.Title, Artist: task.Song.Artist, Duration: 1, Synced: fixtureLRC})
				case "qq":
					if strings.Contains(r.URL.Path, "lyric") {
						json.NewEncoder(w).Encode(map[string]any{"retcode": 0, "lyric": fixtureLRC})
						return
					}
					fmt.Fprint(w, `{"music.search.SearchCgiService":{"data":{"body":{"song":{"list":[{"mid":"fixture","title":"Fixture recording","interval":1,"singer":[{"name":"Madar Tests"}]}]}}}}}`)
				case "netease":
					if strings.Contains(r.URL.Path, "lyric") {
						json.NewEncoder(w).Encode(map[string]any{"lrc": map[string]string{"lyric": fixtureLRC}})
						return
					}
					fmt.Fprint(w, `{"result":{"songs":[{"id":42,"name":"Fixture recording","duration":1000,"artists":[{"name":"Madar Tests"}]}]}}`)
				case "kugou":
					if r.URL.Path == "/download" {
						json.NewEncoder(w).Encode(map[string]string{"content": base64.StdEncoding.EncodeToString([]byte(fixtureLRC))})
						return
					}
					fmt.Fprint(w, `{"candidates":[{"id":"1","accesskey":"fixture","song":"Fixture recording","singer":"Madar Tests","duration":1000}]}`)
				case "musixmatch":
					if r.URL.Path == "/track.search" {
						fmt.Fprint(w, `{"message":{"header":{"status_code":200},"body":{"track_list":[{"track":{"track_id":1,"track_name":"Fixture recording","artist_name":"Madar Tests","track_length":1}}]}}}`)
						return
					}
					json.NewEncoder(w).Encode(map[string]any{"message": map[string]any{"header": map[string]int{"status_code": 200}, "body": map[string]any{"subtitle": map[string]string{"subtitle_body": fixtureLRC}}}})
				}
			}))
			defer server.Close()
			s.lyricTestBases = map[string]string{provider: server.URL}
			r, e := s.remoteLyricsFrom(context.Background(), task.Song, provider)
			if e != nil || r.Synced != fixtureLRC || r.Provider != provider {
				t.Fatalf("provider contract %s: %+v %v", provider, r, e)
			}
			if _, e = s.readLyricCache(task.Song); e != nil {
				t.Fatal("lyrics were not cached", e)
			}
		})
	}
}
func TestTimedLyricsFallbackAndOfflineCache(t *testing.T) {
	s := testStudio(t)
	task := lyricTask(t, s)
	s.state.Preferences.LyricProviders = []string{"lrclib", "qq"}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		switch {
		case strings.Contains(r.URL.Path, "lrclib"):
			json.NewEncoder(w).Encode(LyricsRecord{Title: task.Song.Title, Artist: task.Song.Artist, Duration: 1, Plain: "First test line\nSecond test line"})
		case strings.Contains(r.URL.Path, "lyric"):
			json.NewEncoder(w).Encode(map[string]string{"lyric": fixtureLRC})
		default:
			fmt.Fprint(w, `{"music.search.SearchCgiService":{"data":{"body":{"song":{"list":[{"mid":"fixture","title":"Fixture recording","interval":1,"singer":[{"name":"Madar Tests"}]}]}}}}}`)
		}
	}))
	s.lyricTestBases = map[string]string{"lrclib": server.URL + "/lrclib", "qq": server.URL}
	view, e := s.GetLyrics(task.ID, false)
	if e != nil || !view.Timed || view.Source != "qq" {
		t.Fatalf("timed fallback %+v %v", view, e)
	}
	before := calls.Load()
	server.Close()
	view, e = s.GetLyrics(task.ID, false)
	if e != nil || !view.Timed || calls.Load() != before {
		t.Fatal("offline cache did not prevent network")
	}
	file, _ := s.task(task.ID)
	if b, e := os.ReadFile(file.Lyrics); e != nil || string(b) != fixtureLRC {
		t.Fatal("offline LRC sidecar missing")
	}
}
func TestLyricsExplicitRefreshAndLocalOverride(t *testing.T) {
	s := testStudio(t)
	task := lyricTask(t, s)
	s.state.Preferences.LyricProviders = []string{"lrclib"}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		json.NewEncoder(w).Encode(LyricsRecord{Title: task.Song.Title, Artist: task.Song.Artist, Duration: 1, Synced: fmt.Sprintf("[00:00.10]Version %d\n[00:00.50]Test", n)})
	}))
	defer server.Close()
	s.lyricTestBases = map[string]string{"lrclib": server.URL}
	a, e := s.GetLyrics(task.ID, false)
	if e != nil {
		t.Fatal(e)
	}
	b, e := s.GetLyrics(task.ID, true)
	if e != nil || a.Text == b.Text || calls.Load() != 2 {
		t.Fatal("refresh returned old cache")
	}
	if e = s.SaveLyricsText(task.ID, fixtureLRC, true); e != nil {
		t.Fatal(e)
	}
	view, e := s.GetLyrics(task.ID, false)
	if e != nil || view.Text != fixtureLRC || view.Source != "local" {
		t.Fatal("manual LRC was replaced by old cache")
	}
	if e = s.SaveLyricsText(task.ID, "plain untimed text", true); e == nil {
		t.Fatal("plain text accepted as timed")
	}
}
func TestLyricsRejectWrongRecordingAndMalformedTimings(t *testing.T) {
	song := Song{Title: "Song", Artist: "Artist", Duration: 200}
	if lyricsRecordingMatch(song, "Song (Live)", "Artist", 200) || lyricsRecordingMatch(song, "Song", "Cover Artist", 200) || lyricsRecordingMatch(song, "Song", "Artist", 240) {
		t.Fatal("wrong recording accepted")
	}
	for _, text := range []string{"plain text", "[ar:Artist]\n[ti:Song]", "[99:90]bad\n[99:91]bad", "[50:00.00]wrong duration\n[50:01.00]wrong duration"} {
		if validLRC(text, 200) {
			t.Fatal("invalid timing accepted", text)
		}
	}
}
func TestWordTimingCacheAndRestore(t *testing.T) {
	s := testStudio(t)
	task := lyricTask(t, s)
	if e := s.SaveLyricsText(task.ID, "Original first line\nOriginal second line", false); e != nil {
		t.Fatal(e)
	}
	output := validAlignmentFixture()
	output.Original = "Original first line\nOriginal second line"
	output.OriginalTimed = false
	output.OriginalSource = "local"
	output.LRC = "[00:00.10]Original first line\n[00:00.60]Original second line\n"
	output.Lines[0].Text = "Original first line"
	output.Lines[0].Words[0].Text = "Original"
	output.Lines[0].Words[1].Text = " first"
	output.Lines[1].Text = "Original second line"
	output.Lines[1].Words[0].Text = "Original"
	output.Lines[1].Words[1].Text = " second"
	raw, _ := json.Marshal(output)
	dir := filepath.Join(s.root, "data", "alignment")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, task.ID+"-result.json"), raw, 0600)
	if e := s.ApplyAlignment(task.ID); e != nil {
		t.Fatal(e)
	}
	view, e := s.GetLyrics(task.ID, false)
	if e != nil || len(view.Words) != 2 || !view.Timed {
		t.Fatal("word timings not persisted")
	}
	r := NewStudio(s.root)
	view, e = r.GetLyrics(task.ID, false)
	if e != nil || len(view.Words) != 2 {
		t.Fatal("word timings did not survive restart")
	}
	if e = r.RestoreLyrics(task.ID); e != nil {
		t.Fatal(e)
	}
	view, e = r.GetLyrics(task.ID, false)
	if e != nil || view.Timed || !strings.HasPrefix(view.Text, "Original") {
		t.Fatal("original lyrics were lost")
	}
}
func TestCredentialsEncryptedAndRestored(t *testing.T) {
	s := testStudio(t)
	s.spotifyToken = "fixture-private-access"
	s.spotifyRefresh = "fixture-private-refresh"
	s.spotifyExpiry = time.Now().Add(time.Hour)
	if e := s.SetMusixmatchKey("fixture-private-mxm"); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(filepath.Join(s.root, "data", "credentials.dpapi"))
	if e != nil || strings.Contains(string(b), "fixture-private") {
		t.Fatal("credentials stored as plain text")
	}
	r := NewStudio(s.root)
	if r.spotifyToken != s.spotifyToken || r.musixmatchKey != "fixture-private-mxm" {
		t.Fatal("secure credentials did not restore")
	}
	state, _ := json.Marshal(r.GetState())
	if strings.Contains(string(state), "fixture-private") {
		t.Fatal("private token exposed to UI")
	}
}
func TestPortablePathsAndCorruptState(t *testing.T) {
	s := testStudio(t)
	task := lyricTask(t, s)
	s.state.Preferences.Folder = filepath.Join(s.root, "Downloads")
	s.save()
	newRoot := t.TempDir()
	os.MkdirAll(filepath.Join(newRoot, "data"), 0755)
	b, _ := os.ReadFile(filepath.Join(s.root, "data", "library.json"))
	os.WriteFile(filepath.Join(newRoot, "data", "library.json"), b, 0600)
	r := NewStudio(newRoot)
	if r.state.Jobs[0].Path != filepath.Join(newRoot, "song.flac") || r.state.Preferences.Folder != filepath.Join(newRoot, "Downloads") {
		t.Fatal("portable paths were not rebased")
	}
	_ = task
	os.WriteFile(filepath.Join(newRoot, "data", "library.json"), []byte(`{"jobs":[invalid`), 0600)
	r = NewStudio(newRoot)
	if len(r.state.Jobs) != 0 || !r.state.Preferences.AutoMatch || r.state.StorageError == "" {
		t.Fatal("corrupt state did not recover safely")
	}
	backups, _ := filepath.Glob(filepath.Join(newRoot, "data", "library-corrupt-*.json"))
	if len(backups) != 1 {
		t.Fatal("corrupt state backup missing")
	}
}
func TestClearDASHManifestProtection(t *testing.T) {
	s := testStudio(t)
	for _, xml := range []string{`<MPD><Period><ContentProtection schemeIdUri="x"/></Period></MPD>`, `<not-an-mpd/>`} {
		if _, e := s.saveClearDASH([]byte(xml)); e == nil {
			t.Fatal("invalid/protected DASH accepted")
		}
	}
	if _, e := s.saveClearDASH([]byte(`<MPD><Period><AdaptationSet><Representation><SegmentTemplate initialization="https://example.com/0.mp4" media="https://example.com/$Number$.mp4"/></Representation></AdaptationSet></Period></MPD>`)); e != nil {
		t.Fatal(e)
	}
}
