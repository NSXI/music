package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestV2CloudCredentialsStaySealedAndConflictsRequireReview(t *testing.T) {
	s := testStudio(t)
	v2Task(t, s)
	var mu sync.Mutex
	var body []byte
	etag := ""
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		u, p, ok := r.BasicAuth()
		if !ok || u != "fixture" || p != "private-fixture-password" {
			w.WriteHeader(401)
			return
		}
		if r.Method == "PUT" {
			if etag != "" && r.Header.Get("If-Match") != etag {
				w.WriteHeader(412)
				return
			}
			var value json.RawMessage
			if e := json.NewDecoder(r.Body).Decode(&value); e != nil {
				w.WriteHeader(400)
				return
			}
			body = append([]byte(nil), value...)
			etag = `"fixture-1"`
			w.Header().Set("ETag", etag)
			w.WriteHeader(201)
			return
		}
		w.Header().Set("ETag", etag)
		w.Write(body)
	}))
	defer server.Close()
	s.client = server.Client()
	if e := s.SetCloudSyncConfig(server.URL+"/backup.json", "fixture", "private-fixture-password"); e != nil {
		t.Fatal(e)
	}
	sealed, e := os.ReadFile(filepath.Join(s.root, "data", "cloud-sync.sealed"))
	if e != nil || strings.Contains(string(sealed), "private-fixture-password") {
		t.Fatal("credentials not sealed", e)
	}
	if e = s.UploadCloudBackup(); e != nil {
		t.Fatal(e)
	}
	mu.Lock()
	etag = `"remote-change"`
	mu.Unlock()
	if e = s.UploadCloudBackup(); e == nil {
		t.Fatal("unreviewed remote backup overwritten")
	}
	b, e := s.DownloadCloudBackup()
	if e != nil || b.Schema != 2 {
		t.Fatal(b, e)
	}
	if e = s.UploadCloudBackup(); e != nil {
		t.Fatal(e)
	}
	exported, _ := s.ExportWorkspaceBackup()
	raw, _ := json.Marshal(exported)
	if strings.Contains(string(raw), "private-fixture-password") {
		t.Fatal("secret exported")
	}
	if e = s.SetCloudSyncConfig("http://example.test/backup.json", "", ""); e == nil {
		t.Fatal("insecure endpoint accepted")
	}
}

func TestV2ComponentArchiveRejectsTraversalAndInstallsOnlyOwnedFiles(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "data", "components"), 0700)
	makeZip := func(name string, entries map[string]string) string {
		p := filepath.Join(t.TempDir(), name)
		f, e := os.Create(p)
		if e != nil {
			t.Fatal(e)
		}
		z := zip.NewWriter(f)
		for n, v := range entries {
			w, e := z.Create(n)
			if e != nil {
				t.Fatal(e)
			}
			w.Write([]byte(v))
		}
		if e = z.Close(); e != nil {
			t.Fatal(e)
		}
		f.Close()
		return p
	}
	bad := makeZip("bad.zip", map[string]string{"../outside.txt": "no"})
	if e := installComponentArchive(context.Background(), root, "windows-alignment", bad); e == nil {
		t.Fatal("traversal accepted")
	}
	missing := makeZip("missing.zip", map[string]string{"tools/aligner/MadarAlignV2.exe": "worker"})
	if e := installComponentArchive(context.Background(), root, "windows-alignment", missing); e == nil {
		t.Fatal("missing model accepted")
	}
	ok := makeZip("ok.zip", map[string]string{"tools/aligner/MadarAlignV2.exe": "worker", "tools/aligner/fixture.dll": "dependency", "tools/models/base.pt": "model"})
	if e := installComponentArchive(context.Background(), root, "windows-alignment", ok); e != nil {
		t.Fatal(e)
	}
	if b, e := os.ReadFile(filepath.Join(root, "tools", "models", "base.pt")); e != nil || string(b) != "model" {
		t.Fatal("model not installed", e)
	}
	if _, e := os.Stat(filepath.Join(root, "outside.txt")); !os.IsNotExist(e) {
		t.Fatal("unexpected path created")
	}
}

func TestV2MetadataEditRetainsCachedWordsTranslationsAndHistory(t *testing.T) {
	s := testStudio(t)
	task := v2Task(t, s)
	s.SaveLyricsText(task.ID, "First fixture line\nSecond fixture line", false)
	v, e := s.GetTranslation(task.ID, "ar", "en", false)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.SaveTranslation(task.ID, "ar", "en", v.OriginalHash, []string{"fixture one", "fixture two"}); e != nil {
		t.Fatal(e)
	}
	s.SaveLyricsText(task.ID, "First fixture line\nSecond fixture line", false)
	if e = s.EditLibraryMetadata([]MetadataEdit{{ID: task.ID, Title: "Corrected title", Artist: "Corrected artist", Album: "Fixture", Genres: []string{"Original"}}}); e != nil {
		t.Fatal(e)
	}
	after, e := s.GetTranslation(task.ID, "ar", "en", false)
	if e != nil || !after.Exists || after.Lines[0].Text != "fixture one" {
		t.Fatal(after, e)
	}
	history, e := s.GetLyricHistory(task.ID)
	if e != nil || len(history) == 0 {
		t.Fatal("history lost after tag correction", e)
	}
}

func TestV2SnapshotCopiesMutableDataWithoutChangingStoredLogs(t *testing.T) {
	s := testStudio(t)
	task := v2Task(t, s)
	task.Song.Genres = []string{"Original"}
	task.Log = make([]string, 100)
	for i := range task.Log {
		task.Log[i] = "entry"
	}
	s.state.Jobs[0] = task
	s.state.Preferences.LibraryFolders = []string{"C:/fixture"}
	s.state.Cooldowns = map[string]int64{"fixture": 42}
	result := s.GetState()
	if len(result.Jobs[0].Log) != 50 {
		t.Fatal("UI logs not bounded")
	}
	result.Jobs[0].Song.Genres[0] = "changed"
	result.Jobs[0].Log[0] = "changed"
	result.Preferences.LibraryFolders[0] = "changed"
	result.Cooldowns["fixture"] = 99
	if s.state.Jobs[0].Song.Genres[0] != "Original" || s.state.Jobs[0].Log[50] != "entry" || s.state.Cooldowns["fixture"] != 42 || s.state.Preferences.LibraryFolders[0] != "C:/fixture" {
		t.Fatal("snapshot aliases library")
	}
	logs, e := s.GetTaskLog(task.ID)
	if e != nil || len(logs) != 100 {
		t.Fatal("full logs unavailable", e)
	}
	logs[0] = "changed"
	if s.state.Jobs[0].Log[0] != "entry" {
		t.Fatal("full logs alias source")
	}
}

func TestV2PortableBackupIncludesExplicitSourceTranslations(t *testing.T) {
	s := testStudio(t)
	task := v2Task(t, s)
	if e := s.SaveLyricsText(task.ID, "First original line\nSecond original line", false); e != nil {
		t.Fatal(e)
	}
	tr, e := s.GetTranslation(task.ID, "ar", "en", false)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.SaveTranslation(task.ID, "ar", "en", tr.OriginalHash, []string{"fixture first translation", "fixture second translation"}); e != nil {
		t.Fatal(e)
	}
	b, e := s.ExportWorkspaceBackup()
	if e != nil {
		t.Fatal(e)
	}
	other := testStudio(t)
	copyTask := task
	copyTask.ID = "backup-receiver"
	other.state.Jobs = []Task{copyTask}
	if _, e = other.RestoreWorkspaceBackup(b, false); e != nil {
		t.Fatal(e)
	}
	after, e := other.GetTranslation(copyTask.ID, "ar", "en", false)
	if e != nil || !after.Exists || after.Lines[0].Text != "fixture first translation" {
		t.Fatal("explicit-source translation omitted", after, e)
	}
}

func TestV2TwoTransfersAndIndividualPauseAreBounded(t *testing.T) {
	s := testStudio(t)
	audio := fixtureAudio(t, s)
	s.state.Preferences.AutoMatch = false
	s.state.Preferences.AutoTranslate = false
	s.state.Preferences.Lyrics = false
	started := make(chan string, 4)
	release := make(chan struct{})
	defer close(release)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started <- r.URL.Path
		w.Header().Set("Content-Type", "audio/flac")
		w.Header().Set("Content-Length", fmt.Sprint(len(audio)))
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
			return
		case <-release:
		}
		w.Write(audio)
	}))
	defer server.CloseClientConnections()
	defer server.Close()
	for i := 0; i < 3; i++ {
		id := fmt.Sprint("parallel-", i)
		song := Song{ID: id, Title: id, Artist: "Fixture", Provider: "direct", URL: server.URL + "/" + id, AudioURL: server.URL + "/" + id}
		s.state.Jobs = append(s.state.Jobs, Task{ID: id, Song: song, Status: "queued", Log: []string{}})
	}
	w, _ := s.GetWorkspace()
	if _, e := s.SaveWorkspace(w.Revision, map[string]json.RawMessage{}, DownloadPolicy{Parallel: 2}); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})
	go func() { s.loop(ctx); close(finished) }()
	defer func() {
		cancel()
		s.mu.Lock()
		for _, active := range s.activeDownloads {
			active.cancel()
		}
		s.mu.Unlock()
		select {
		case <-finished:
		case <-time.After(5 * time.Second):
			t.Error("workers failed to stop")
		}
	}()
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("two transfers did not start")
		}
	}
	select {
	case <-started:
		t.Fatal("parallel limit exceeded")
	case <-time.After(80 * time.Millisecond):
	}
	if e := s.Control("parallel-0", "pause"); e != nil {
		t.Fatal(e)
	}
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		job, _ := s.task("parallel-0")
		if job.Status == "paused" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	job, _ := s.task("parallel-0")
	other, _ := s.task("parallel-1")
	if job.Status != "paused" || other.Status != "working" {
		t.Fatal("pause affected wrong transfer", job.Status, other.Status)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("freed slot not reused")
	}
}

func BenchmarkV2Snapshot(b *testing.B) {
	sample := Snapshot{Jobs: make([]Task, 3000)}
	for i := range sample.Jobs {
		sample.Jobs[i] = Task{ID: fmt.Sprint(i), Song: Song{Title: "Fixture", Artist: "Benchmark", Genres: []string{"Original"}}, Log: []string{"a", "b"}}
	}
	b.Run("DirectClone", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			cloneSnapshot(sample)
		}
	})
	b.Run("JSONBaseline", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			raw, _ := json.Marshal(sample)
			var result Snapshot
			json.NewDecoder(strings.NewReader(string(raw))).Decode(&result)
		}
	})
}
