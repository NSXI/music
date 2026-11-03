package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestAMLLExplicitWordsAndOfflineIndex(t *testing.T) {
	s := testStudio(t)
	song := Song{ID: "own-amll-fixture", Title: "Own fixture", Artist: "Nawa Tests", Duration: 10}
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if strings.HasSuffix(r.URL.Path, ".jsonl") {
			json.NewEncoder(w).Encode(map[string]any{"metadata": [][]any{{"musicName", []string{song.Title}}, {"artists", []string{song.Artist}}}, "rawLyricFile": "own-fixture.ttml"})
		} else {
			w.Write([]byte(`<tt><body><div><p begin="1" end="3"><span begin="1" end="3"><span begin="1" end="2">مرحبا</span> <span begin="2" end="3">بكم</span></span><span xmlns:ttm="urn:test" ttm:role="x-translation">Hello all</span></p><p begin="4" end="5">Fixture two</p></div></body></tt>`))
		}
	}))
	defer server.Close()
	s.sourceTestBases = map[string]string{"amll": server.URL}
	r, e := s.remoteLyricsFrom(context.Background(), song, "amll")
	if e != nil || len(r.Words) != 2 || len(r.Words[0].Words) != 2 || r.Words[0].Words[1].Start != 2 || strings.Contains(r.Plain, "Hello") {
		t.Fatalf("explicit source timing/text: %+v %v", r, e)
	}
	before := hits.Load()
	server.Close()
	cached, e := s.remoteLyricsFrom(context.Background(), song, "auto")
	if e != nil || !cached.Cached || hits.Load() != before || len(cached.Words[0].Words) != 2 {
		t.Fatal("offline word cache lost", e)
	}
}
func TestYRCImportExactWindows(t *testing.T) {
	s := testStudio(t)
	task := lyricTask(t, s)
	task.Song.Duration = 10
	s.edit(task.ID, func(j *Task) { j.Song.Duration = 10 })
	raw := "[1000,2000](1000,900,0)こんにちは(2000,1000,0)世界\n[4000,1000](4000,1000,0)Привет"
	if e := s.SaveImportedLyrics(task.ID, raw, "yrc"); e != nil {
		t.Fatal(e)
	}
	c, e := s.readLyricCache(task.Song)
	if e != nil || c.Record.Provider != "local" || len(c.Words) != 2 || c.Words[0].Words[0].End != 1.9 || c.Words[0].Text != "こんにちは世界" {
		t.Fatalf("YRC data lost: %+v %v", c, e)
	}
	for _, bad := range []string{strings.Replace(raw, "1000,900", "0,900", 1), strings.Replace(raw, "4000,1000,0", "4000,9000,0", 1)} {
		if _, e := decodeYRC(bad, task.Song); e == nil {
			t.Fatal("accepted out-of-window word")
		}
	}
}
func TestSearchRelevanceAcrossScripts(t *testing.T) {
	for _, q := range []string{"A recording", "こんにちは世界", "أغنية جميلة", "Привет"} {
		exact := Song{Title: q, Artist: "Test"}
		unrelated := Song{Title: "Other", Artist: "Artist", Codec: "FLAC"}
		if musicSearchScore(q, exact) <= musicSearchScore(q, unrelated) {
			t.Fatal("lossless badge outranked relevant result", q)
		}
	}
}

func TestLateAutomaticLyricsKeepManualEdit(t *testing.T) {
	s := testStudio(t)
	task := lyricTask(t, s)
	if e := s.SaveLyricsText(task.ID, fixtureLRC, true); e != nil {
		t.Fatal(e)
	}
	r, e := s.cacheRemoteLyrics(task.Song, LyricsRecord{Provider: "amll", Plain: "late remote response"}, true)
	if e != nil || !r.Cached || r.Provider != "local" || r.Synced != fixtureLRC {
		t.Fatal("automatic fetch replaced manual timing", e, r.Provider)
	}
}
func TestPairedReleaseVersionMatchesCore(t *testing.T) {
	raw, e := os.ReadFile("release.json")
	if e != nil {
		t.Fatal(e)
	}
	var release struct {
		Version string `json:"version"`
		Code    int    `json:"versionCode"`
	}
	if e = json.Unmarshal(raw, &release); e != nil || release.Version != appReleaseVersion || release.Code != appReleaseCode {
		t.Fatal("package/core release identity differs", release, e)
	}
}
func TestArchiveFLACSearchRestrictsActualFiles(t *testing.T) {
	s := testStudio(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/advancedsearch.php" {
			if !strings.Contains(r.URL.Query().Get("q"), "format:FLAC") {
				t.Error("missing FLAC query")
			}
			w.Write([]byte(`{"response":{"docs":[{"identifier":"own","title":"Fixture"}]}}`))
			return
		}
		w.Write([]byte(`{"metadata":{"title":"Own fixture","creator":"Nawa"},"files":[{"name":"one.flac","title":"One","size":"500","length":"10"},{"name":"two.mp3","title":"Two","size":"100","length":"10"},{"name":"private.flac","title":"Private","private":true}]}`))
	}))
	defer server.Close()
	s.sourceTestBases = map[string]string{"archive": server.URL}
	r, e := s.providerSearch(context.Background(), "archive-flac", "Fixture")
	if e != nil || len(r.Tracks) != 1 || r.Tracks[0].Codec != "FLAC" || r.Tracks[0].QualityVerified {
		t.Fatalf("wrong lossless catalog: %+v %v", r, e)
	}
}
func TestDownloadReadyWhileMetadataPending(t *testing.T) {
	s := testStudio(t)
	task := testTask(s, "background-metadata")
	path := filepath.Join(s.root, "original.wav")
	if e := os.WriteFile(path, fixtureAudio(t, s), 0600); e != nil {
		t.Fatal(e)
	}
	q, e := s.probe(context.Background(), path)
	if e != nil {
		t.Fatal(e)
	}
	// No metadata worker is required for an audio file to become playable.
	started := time.Now()
	if e = s.commitAudio(context.Background(), task, task.Song, path, q, s.GetState().Preferences); e != nil {
		t.Fatal(e)
	}
	ready, ok := s.task(task.ID)
	if !ok || ready.Status != "done" || !ready.MetadataPending || ready.Path == "" || time.Since(started) > 5*time.Second {
		t.Fatalf("audio held behind metadata: %+v", ready)
	}
	next := NewStudio(s.root)
	restored, _ := next.task(task.ID)
	if !restored.MetadataPending || restored.Status != "done" {
		t.Fatal("pending metadata/download lost on restart")
	}
}
