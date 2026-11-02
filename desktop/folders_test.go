package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func waitFolder(t *testing.T, s *Studio) FolderImportState {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		state := s.GetState().FolderImport
		if !state.Running {
			return state
		}
		time.Sleep(15 * time.Millisecond)
	}
	t.Fatal("folder scan did not finish")
	return FolderImportState{}
}
func TestFolderEnumerationNestedAndBounded(t *testing.T) {
	s := testStudio(t)
	dir := t.TempDir()
	nested := filepath.Join(dir, "موسيقى")
	os.MkdirAll(nested, 0755)
	for path, body := range map[string]string{filepath.Join(dir, "one.FLAC"): "fixture", filepath.Join(nested, "two.mp3"): "fixture", filepath.Join(nested, "cover.jpg"): "image", filepath.Join(nested, "lyrics.lrc"): "text"} {
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	paths := s.collectFolderAudio(context.Background(), []string{dir, nested})
	if len(paths) != 2 {
		t.Fatalf("wanted two distinct audio paths, got%v", paths)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if paths := s.collectFolderAudio(ctx, []string{dir}); len(paths) != 0 {
		t.Fatal("cancelled scan found paths")
	}
	if _, err := s.ImportLibraryFolders([]string{""}); err == nil {
		t.Fatal("empty path accepted")
	}
	if _, err := s.ImportLibraryFolders([]string{filepath.Join(dir, "missing")}); err == nil {
		t.Fatal("missing path accepted")
	}
}
func TestFolderImportRescanPreservesFiles(t *testing.T) {
	s := testStudio(t)
	s.state.Preferences.AutoAlign = false
	audio := fixtureAudio(t, s)
	dir := t.TempDir()
	nested := filepath.Join(dir, "Album")
	os.MkdirAll(nested, 0755)
	one, two := filepath.Join(dir, "one.flac"), filepath.Join(nested, "duplicate.flac")
	os.WriteFile(one, audio, 0600)
	os.WriteFile(two, audio, 0600)
	if _, err := s.ImportLibraryFolders([]string{dir}); err != nil {
		t.Fatal(err)
	}
	state := waitFolder(t, s)
	if state.Added != 1 || state.Skipped != 1 || state.Failed != 0 {
		t.Fatalf("unexpected folder result %+v", state)
	}
	if len(s.GetState().Jobs) != 1 || len(s.GetState().Preferences.LibraryFolders) != 1 {
		t.Fatal("folder or track not retained")
	}
	if _, err := s.RescanLibraryFolders(); err != nil {
		t.Fatal(err)
	}
	state = waitFolder(t, s)
	if state.Added != 0 || state.Skipped != 2 {
		t.Fatalf("rescan duplicated tracks %+v", state)
	}
	reloaded := NewStudio(s.root)
	if len(reloaded.GetState().Preferences.LibraryFolders) != 1 || len(reloaded.GetState().Jobs) != 1 {
		t.Fatal("restart lost folder/library")
	}
	if err := s.RemoveLibraryFolder(dir); err != nil {
		t.Fatal(err)
	}
	if len(s.GetState().Jobs) != 1 {
		t.Fatal("removing folder deleted tracks")
	}
	for _, path := range []string{one, two} {
		if _, err := os.Stat(path); err != nil {
			t.Fatal("source audio removed")
		}
	}
}
func TestMetadataKeysEncryptedAndNotInSnapshot(t *testing.T) {
	s := testStudio(t)
	if err := s.SetMetadataKey("fanart", "fixture-secret-api-token"); err != nil {
		t.Fatal(err)
	}
	snapshot := s.GetState()
	if !snapshot.MetadataConfigured["fanart"] {
		t.Fatal("configured flag absent")
	}
	raw, _ := json.Marshal(snapshot)
	if strings.Contains(string(raw), "fixture-secret-api-token") {
		t.Fatal("secret leaked through snapshot")
	}
	sealed, err := os.ReadFile(filepath.Join(s.root, "data", "credentials.dpapi"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(sealed), "fixture-secret-api-token") {
		t.Fatal("secret stored in clear text")
	}
	restored := NewStudio(s.root)
	if restored.metadataKey("fanart") != "fixture-secret-api-token" {
		t.Fatal("encrypted key did not round trip")
	}
	if err := restored.SetMetadataKey("fanart", ""); err != nil {
		t.Fatal(err)
	}
	if restored.GetState().MetadataConfigured["fanart"] {
		t.Fatal("key clear failed")
	}
	if err := s.SetMetadataKey("invalid provider", "key"); err == nil {
		t.Fatal("invalid provider key accepted")
	}
	if s.GetState().Preferences.LyricProviders == nil {
		t.Fatal("missing source defaults")
	}
	for _, id := range s.GetState().Preferences.LyricProviders {
		if id == "audd" {
			t.Fatal("metered source enabled without opt-in")
		}
	}
}
