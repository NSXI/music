package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestV2ActualPeerTransferVerifiesAudioAndRemapsLyrics(t *testing.T) {
	sender := testStudio(t)
	task := v2Task(t, sender)
	task.Song.Duration = 1.6
	b := fixtureAudio(t, sender)
	os.WriteFile(task.Path, b, 0600)
	task.Hash = fileHash(task.Path)
	task.Bytes = int64(len(b))
	task.Total = task.Bytes
	sender.state.Jobs[0] = task
	sender.SaveLyricsText(task.ID, "[00:00.100]Original fixture\n[00:00.700]Local transfer", true)
	w, _ := sender.GetWorkspace()
	fav, _ := json.Marshal([]string{task.ID})
	sender.SaveWorkspace(w.Revision, map[string]json.RawMessage{"favorites": fav}, DownloadPolicy{})
	link := &deviceLink{token: strings.Repeat("b", 48), expires: time.Now().Add(time.Minute), shared: map[string]bool{task.ID: true}, playback: map[string]any{}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { sender.serveDeviceLink(link, w, r) }))
	defer server.Close()
	receiver := testStudio(t)
	if e := receiver.pullDeviceLibrary(context.Background(), server.URL+"/#"+link.token, []string{task.ID}, true); e != nil {
		t.Fatal(e)
	}
	jobs := receiver.GetState().Jobs
	if len(jobs) != 1 || jobs[0].Hash != task.Hash {
		t.Fatal("peer file not verified", jobs)
	}
	doc, e := receiver.GetLyricDocument(jobs[0].ID)
	if e != nil || !strings.Contains(doc.Text, "Local transfer") {
		t.Fatal("peer lyrics not remapped", doc, e)
	}
	restored, _ := receiver.GetWorkspace()
	if !strings.Contains(string(restored.UI["favorites"]), jobs[0].ID) {
		t.Fatal("peer favorite identity lost")
	}
	if _, e = os.Stat(task.Path); e != nil {
		t.Fatal("sender file changed")
	}
}
func TestV2ReviewedQualityUpgradeKeepsIDsOriginalFileAndLyrics(t *testing.T) {
	s := testStudio(t)
	original := fixtureAudio(t, s)
	old := v2Task(t, s)
	old.Path = filepath.Join(s.root, "original-lossy.mp3")
	if output, e := s.command(context.Background(), "ffmpeg", "-v", "error", "-nostdin", "-y", "-i", filepath.Join(s.root, "fixture.flac"), "-c:a", "libmp3lame", "-b:a", "128k", old.Path).CombinedOutput(); e != nil {
		t.Fatal(e, string(output))
	}
	old.Hash = fileHash(old.Path)
	var probeError error
	old.Quality, probeError = s.probe(context.Background(), old.Path)
	if probeError != nil {
		t.Fatal(probeError)
	}
	old.Song.Duration = 1
	s.state.Jobs[0] = old
	s.SaveLyricsText(old.ID, "[00:00.100]Keep original words\n[00:00.700]Keep local timing", true)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/flac")
		w.Write(original)
	}))
	defer server.Close()
	candidate := old.Song
	candidate.ID = "verified-source-candidate"
	candidate.Provider = "direct"
	candidate.URL = server.URL
	candidate.AudioURL = server.URL
	s.known[candidate.ID] = candidate
	s.state.Preferences.AutoMatch = false
	upgradeID, e := s.QueueTrackUpgrade(old.ID, candidate.ID)
	if e != nil {
		t.Fatal(e)
	}
	upgrade, _ := s.task(upgradeID)
	if e = s.execute(context.Background(), upgrade); e != nil {
		t.Fatal(e)
	}
	s.edit(upgradeID, func(t *Task) { t.Status = "done" })
	replacement, _ := s.task(upgradeID)
	if replacement.Path == old.Path {
		t.Fatal("upgrade overwrote original before review")
	}
	if e = s.ApplyTrackUpgrade(upgradeID); e != nil {
		t.Fatal(e)
	}
	state := s.GetState()
	if len(state.Jobs) != 1 || state.Jobs[0].ID != old.ID || !state.Jobs[0].Quality.Lossless || state.Jobs[0].Path == old.Path {
		t.Fatal("upgrade identity or quality lost", state.Jobs)
	}
	if fileHash(old.Path) != old.Hash {
		t.Fatal("original audio file changed")
	}
	doc, e := s.GetLyricDocument(old.ID)
	if e != nil || !strings.Contains(doc.Text, "Keep original words") {
		t.Fatal(doc, e)
	}
	backups, _ := filepath.Glob(filepath.Join(s.root, "data", "audio-upgrades", old.ID+"-before-*.json"))
	if len(backups) != 1 {
		t.Fatal("upgrade recovery record missing")
	}
}
func TestV2ImportedUIRejectsBrokenTypesAndUnsafeDesignValues(t *testing.T) {
	for k, v := range map[string]string{"favorites": "true", "eq": "[1]", "fsMedium": `"square\"><img>"`, "savedDesigns": `{"song":{"fsBackground":"unknown"}}`, "speed": "0", "themePresets": `[{"name":"x","design":{"fsVariants":{"cd":"unknown"}}}]`} {
		if _, e := cleanWorkspaceUI(map[string]json.RawMessage{k: json.RawMessage(v)}); e == nil {
			t.Fatal("invalid imported UI accepted", k)
		}
	}
}

func TestV2ScheduleUsesExplicitDeviceTimezone(t *testing.T) {
	s := testStudio(t)
	s.SetTransferTimezoneOffset(19800)
	at := time.Date(2026, 11, 4, 20, 0, 0, 0, time.UTC)
	local := s.transferLocalTime(at)
	if local.Hour() != 1 || local.Day() != 5 {
		t.Fatal("device timezone ignored", local)
	}
	s.SetTransferTimezoneOffset(1000000)
	if s.transferLocalTime(at).Hour() != 1 {
		t.Fatal("invalid timezone offset accepted")
	}
}
