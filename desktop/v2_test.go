package main

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func v2Task(t *testing.T, s *Studio) Task {
	t.Helper()
	task := lyricTask(t, s)
	task.Status = "done"
	task.Song.Duration = 12
	s.state.Jobs[0] = task
	s.state.Preferences.AutoTranslate = false
	return task
}
func TestV2WorkspaceOptimisticAndCredentialBoundary(t *testing.T) {
	s := testStudio(t)
	w, e := s.GetWorkspace()
	if e != nil || w.Schema != 2 {
		t.Fatal(w, e)
	}
	ui := map[string]json.RawMessage{"theme": json.RawMessage(`"noir"`), "spotifyToken": json.RawMessage(`"must-not-export"`)}
	next, e := s.SaveWorkspace(w.Revision, ui, DownloadPolicy{StartHour: 22, EndHour: 6})
	if e != nil || next.Revision != 1 || len(next.UI) != 1 {
		t.Fatal(next, e)
	}
	if _, e = s.SaveWorkspace(0, ui, DownloadPolicy{}); e == nil {
		t.Fatal("stale workspace replaced newer settings")
	}
	restored, e := NewStudio(s.root).GetWorkspace()
	if e != nil || string(restored.UI["theme"]) != `"noir"` {
		t.Fatal(restored, e)
	}
	for _, h := range []int{22, 23, 0, 5} {
		if !downloadAllowedAt(next.Downloads, time.Date(2026, 11, 4, h, 0, 0, 0, time.Local)) {
			t.Fatal("overnight window rejected", h)
		}
	}
	for _, h := range []int{6, 12, 21} {
		if downloadAllowedAt(DownloadPolicy{Scheduled: true, StartHour: 22, EndHour: 6}, time.Date(2026, 11, 4, h, 0, 0, 0, time.Local)) {
			t.Fatal("out-of-window download", h)
		}
	}
}
func TestV2PlaybackSessionAndListeningSurviveRestart(t *testing.T) {
	s := testStudio(t)
	task := v2Task(t, s)
	if e := s.SavePlaybackSession(PlaybackSession{TaskID: task.ID, Position: 99, Queue: []string{task.ID, task.ID, "missing"}}); e != nil {
		t.Fatal(e)
	}
	if e := s.RecordListening(task.ID, 31, true); e != nil {
		t.Fatal(e)
	}
	if e := s.SavePlaybackSession(PlaybackSession{TaskID: task.ID, Position: math.NaN()}); e == nil {
		t.Fatal("NaN session")
	}
	w, e := NewStudio(s.root).GetWorkspace()
	if e != nil || w.Playback.Position != 12 || len(w.Playback.Queue) != 1 || w.Listening[task.ID].Plays != 1 || w.Listening[task.ID].Seconds != 31 {
		t.Fatal(w, e)
	}
}
func TestV2LyricEditorUnicodePartialTimingAndHistory(t *testing.T) {
	s := testStudio(t)
	task := v2Task(t, s)
	if e := s.SaveLyricsText(task.ID, "[00:01.000]أهلًا 世界\n[00:07.000]Привет world", true); e != nil {
		t.Fatal(e)
	}
	doc, e := s.GetLyricDocument(task.ID)
	if e != nil {
		t.Fatal(e)
	}
	stale := doc
	doc.Lines = []WordLine{{Start: 1, End: 5, Text: "أهلًا 世界", Words: []TimedWord{{Text: "أهلًا", Start: 1, End: 2, Probability: 0}, {Text: "世界", Start: 3, End: 5, Probability: 0}}}, {Start: 7, End: 11, Text: "Привет world"}}
	next, e := s.SaveLyricDocument(doc)
	if e != nil || len(next.Lines) != 2 || len(next.Lines[0].Words) != 2 || len(next.Lines[1].Words) != 0 {
		t.Fatal(next, e)
	}
	if _, e = s.SaveLyricDocument(stale); e == nil {
		t.Fatal("stale lyrics overwrote current revision")
	}
	history, e := s.GetLyricHistory(task.ID)
	if e != nil || len(history) != 1 {
		t.Fatal(history, e)
	}
	restored, e := s.RestoreLyricRevision(task.ID, history[0].ID, next.Revision)
	if e != nil || !strings.Contains(restored.Text, "Привет") {
		t.Fatal(restored, e)
	}
	if _, e = s.RestoreLyricRevision(task.ID, "../not-a-revision", restored.Revision); e == nil {
		t.Fatal("history path escape")
	}
}
func TestV2LyricEditorRejectsOverlapsAndKeepsOriginal(t *testing.T) {
	s := testStudio(t)
	task := v2Task(t, s)
	s.SaveLyricsText(task.ID, "First line\nSecond line", false)
	doc, _ := s.GetLyricDocument(task.ID)
	old := doc
	doc.Lines = []WordLine{{Start: 1, End: 4, Text: "First line", Words: []TimedWord{{Text: "First", Start: 1, End: 3}, {Text: "line", Start: 2, End: 4}}}, {Start: 5, End: 7, Text: "Second line"}}
	if _, e := s.SaveLyricDocument(doc); e == nil {
		t.Fatal("overlapping edit accepted")
	}
	after, _ := s.GetLyricDocument(task.ID)
	if after.Revision != old.Revision {
		t.Fatal("invalid edit changed original")
	}
}
func TestV2RangeReviewMergesOnlySelectedLines(t *testing.T) {
	s := testStudio(t)
	task := v2Task(t, s)
	s.SaveLyricsText(task.ID, "[00:01.000]First line\n[00:07.000]Second line", true)
	doc, _ := s.GetLyricDocument(task.ID)
	p := RangeAlignmentPreview{TaskID: task.ID, Revision: doc.Revision, Start: 0, End: 6, Lines: []WordLine{{Start: 1.2, End: 4, Text: "First line", Words: []TimedWord{{Text: "First", Start: 1.2, End: 2}, {Text: "line", Start: 2.1, End: 4}}}}}
	b, _ := json.Marshal(p)
	writeChanged(filepath.Join(s.root, "data", "range-alignment", task.ID+".json"), b)
	next, e := s.ApplyRangeAlignment(task.ID)
	if e != nil || len(next.Lines) != 2 || next.Lines[0].Start != 1.2 || next.Lines[1].Start != 7 || len(next.Lines[1].Words) != 0 {
		t.Fatal(next, e)
	}
	if _, e = s.ApplyRangeAlignment(task.ID); e == nil {
		t.Fatal("stale range applied twice")
	}
}
func TestV2PortableBackupRemapsIDsAndPreservesManualEdits(t *testing.T) {
	s := testStudio(t)
	task := v2Task(t, s)
	task.Song.URL = "https://example.test/private?token=do-not-export"
	task.Path = "C:/private/music/song.flac"
	s.state.Jobs[0] = task
	s.SaveLyricsText(task.ID, "First saved line\nSecond saved line", false)
	w, _ := s.GetWorkspace()
	fav, _ := json.Marshal([]string{task.ID})
	s.SaveWorkspace(w.Revision, map[string]json.RawMessage{"favorites": fav, "theme": json.RawMessage(`"noir"`)}, DownloadPolicy{})
	backup, e := s.ExportWorkspaceBackup()
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(backup)
	if strings.Contains(string(raw), "do-not-export") || strings.Contains(string(raw), "C:/private") {
		t.Fatal("backup leaked provider URL or local audio path")
	}
	receiver := testStudio(t)
	copyTask := task
	copyTask.ID = "device-local-id"
	copyTask.Song.ID = "device-song-id"
	copyTask.Path = filepath.Join(receiver.root, "local.flac")
	receiver.state.Jobs = []Task{copyTask}
	receiver.SaveLyricsText(copyTask.ID, "My manual line\nMy other line", false)
	summary, e := receiver.RestoreWorkspaceBackup(backup, false)
	if e != nil || summary.Matched != 1 || summary.Lyrics != 0 {
		t.Fatal(summary, e)
	}
	r, e := receiver.GetWorkspace()
	if e != nil || !strings.Contains(string(r.UI["favorites"]), "device-local-id") {
		t.Fatal(r, e)
	}
	doc, _ := receiver.GetLyricDocument(copyTask.ID)
	if !strings.Contains(doc.Text, "My manual") {
		t.Fatal("manual cache replaced without explicit selection")
	}
}
func TestV2DeviceLinkAuthAndCommands(t *testing.T) {
	s := testStudio(t)
	task := v2Task(t, s)
	token := strings.Repeat("a", 48)
	link := &deviceLink{token: token, expires: time.Now().Add(time.Minute), shared: map[string]bool{task.ID: true}, playback: map[string]any{}}
	s.link = link
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { s.serveDeviceLink(link, w, r) }))
	defer server.Close()
	res, e := http.Get(server.URL + "/manifest")
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	if res.StatusCode != 403 {
		t.Fatal("unauthenticated library access", res.StatusCode)
	}
	raw := server.URL + "/#" + token
	if e = s.SendDeviceCommand(raw, "seek", 3); e != nil {
		t.Fatal(e)
	}
	commands := s.PollDeviceCommands("Title", "Artist", 1, 12, false)
	if len(commands) != 1 || commands[0].Position != 3 {
		t.Fatal(commands)
	}
	if len(s.PollDeviceCommands("", "", 0, 0, true)) != 0 {
		t.Fatal("remote command replay")
	}
	for _, bad := range []string{"https://example.com/#" + token, "http://127.0.0.1:80/#short", "http://user:pass@127.0.0.1:80/#" + token, "http://169.254.169.254:80/#" + token} {
		if _, _, e = parseDeviceLink(bad); e == nil {
			t.Fatal("unsafe peer URL", bad)
		}
	}
	link.expires = time.Now().Add(-time.Second)
	if _, e = s.PreviewDeviceLibrary(raw); e == nil {
		t.Fatal("expired pairing used")
	}
}
func TestV2ActualWaveformLoudnessPitchAndRelink(t *testing.T) {
	s := testStudio(t)
	b := fixtureAudio(t, s)
	path := filepath.Join(s.root, "audio.flac")
	os.WriteFile(path, b, 0600)
	if n, e := s.importPaths(context.Background(), []string{path}); e != nil || n != 1 {
		t.Fatal(n, e)
	}
	task := s.GetState().Jobs[0]
	w, e := s.GetWaveform(task.ID)
	if e != nil || len(w.Peaks) == 0 || w.Duration < .9 {
		t.Fatal(w, e)
	}
	l, e := s.AnalyzeLoudness(task.ID)
	if e != nil || l.TargetLUFS != -18 || math.IsNaN(l.GainDB) {
		t.Fatal(l, e)
	}
	p, e := s.PreparePitchPlayback(task.ID, 3)
	if e != nil || !strings.HasPrefix(p.URL, "/pitch/") {
		t.Fatal(p, e)
	}
	moved := filepath.Join(s.root, "moved.flac")
	if e = os.Rename(path, moved); e != nil {
		t.Fatal(e)
	}
	if _, e = s.importPaths(context.Background(), []string{moved}); e != nil {
		t.Fatal(e)
	}
	state := s.GetState()
	if len(state.Jobs) != 1 || state.Jobs[0].ID != task.ID || state.Jobs[0].Path != moved {
		t.Fatal("relink lost stable identity", state.Jobs)
	}
}
func TestV2SearchGroupingKeepsRecordingVersionsSeparate(t *testing.T) {
	a := Song{Title: "Fixture", Artist: "Tests", Duration: 100}
	b := a
	b.ID = "other"
	c := a
	c.Title += " (Live)"
	d := a
	d.Duration = 170
	groups := groupSearchSongs([]Song{a, b, c, d})
	if len(groups) != 3 || len(groups[0]) != 2 {
		t.Fatal(groups)
	}
	if sameSong(a, c) {
		t.Fatal("live version auto-selected")
	}
}
