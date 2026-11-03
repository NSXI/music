package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBestLyricsPrefersWordsOverEarlierLineProvider(t *testing.T) {
	s := testStudio(t)
	task := lyricTask(t, s)
	rich, _ := newExtraLyricsServer(t, "betterlyrics", false)
	defer rich.Close()
	lineCalls := 0
	line := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lineCalls++
		json.NewEncoder(w).Encode(LyricsRecord{Title: task.Song.Title, Artist: task.Song.Artist, Duration: 1, Synced: fixtureLRC})
	}))
	defer line.Close()
	s.state.Preferences.LyricProviders = []string{"lrclib", "betterlyrics"}
	s.sourceTestBases = map[string]string{"betterlyrics": rich.URL}
	s.lyricTestBases = map[string]string{"lrclib": line.URL}
	r, e := s.remoteLyricsFrom(context.Background(), task.Song, "auto")
	if e != nil || r.Provider != "betterlyrics" || lyricRecordRank(r, task.Song) != 4 || lineCalls != 0 {
		t.Fatalf("best source order/quality: %s %v %d", r.Provider, e, lineCalls)
	}
}
func TestBestLyricsRetainsLineFallbackWhenWordServiceFails(t *testing.T) {
	s := testStudio(t)
	task := lyricTask(t, s)
	failed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "unavailable", 503) }))
	defer failed.Close()
	line := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(LyricsRecord{Title: task.Song.Title, Artist: task.Song.Artist, Duration: 1, Synced: fixtureLRC})
	}))
	defer line.Close()
	s.state.Preferences.LyricProviders = []string{"betterlyrics", "lrclib"}
	s.sourceTestBases = map[string]string{"betterlyrics": failed.URL}
	s.lyricTestBases = map[string]string{"lrclib": line.URL}
	r, e := s.remoteLyricsFrom(context.Background(), task.Song, "auto")
	if e != nil || r.Provider != "lrclib" || lyricRecordRank(r, task.Song) != 2 {
		t.Fatal("valid line fallback lost", r.Provider, e)
	}
}
func TestTimingAnalysisOnlyStartsOnDemand(t *testing.T) {
	s := testStudio(t)
	s.state.Preferences.AutoAlign = true
	s.state.AlignmentQueue = []string{"old-auto-task"}
	s.save()
	restored := NewStudio(s.root)
	if restored.GetState().Preferences.AutoAlign || len(restored.GetState().AlignmentQueue) != 0 {
		t.Fatal("old automatic work restored")
	}
	p := restored.GetState().Preferences
	p.AutoAlign = true
	if e := restored.SetPreferences(p); e != nil {
		t.Fatal(e)
	}
	restored.autoAlign("any")
	if restored.GetState().Preferences.AutoAlign || restored.GetState().Alignment.Running || len(restored.GetState().AlignmentQueue) > 0 {
		t.Fatal("timing started without a user command")
	}
}
