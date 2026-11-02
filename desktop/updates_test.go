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
)

func sampleRelease() UpdateManifest {
	h := strings.Repeat("a", 64)
	return UpdateManifest{Schema: 1, AppID: "dev.nawa.music", Version: "1.3.0", VersionCode: appReleaseCode + 1, SharedRevision: h, ReleaseURL: "https://github.com/NSXI/music/releases/tag/v1.3.0", Assets: map[string]UpdateAsset{"windows-x64": {URL: "https://example.org/pc.zip", SHA256: h, Bytes: 100}, "android-arm64": {URL: "https://example.org/app.apk", SHA256: h, Bytes: 90}}}
}
func TestUpdateManifestRequiresBothPlatformsAndRealHashes(t *testing.T) {
	m := sampleRelease()
	if e := validateUpdateManifest(m); e != nil {
		t.Fatal(e)
	}
	delete(m.Assets, "android-arm64")
	if validateUpdateManifest(m) == nil {
		t.Fatal("partial release accepted")
	}
	m = sampleRelease()
	a := m.Assets["android-arm64"]
	a.SHA256 = "fake"
	m.Assets["android-arm64"] = a
	if validateUpdateManifest(m) == nil {
		t.Fatal("fake checksum accepted")
	}
	m = sampleRelease()
	m.AppID = "another-app"
	if validateUpdateManifest(m) == nil {
		t.Fatal("wrong application accepted")
	}
}
func TestUpdateURLsCannotRunLocalExecutables(t *testing.T) {
	for _, address := range []string{"file:///C:/app.exe", "javascript:alert(1)", "http://example.org/app", "https://user:secret@example.org/app"} {
		if safeUpdateURL(address) {
			t.Fatal(address)
		}
	}
}
func TestSharedFeedSelectsNewerReleaseAndPreservesLibrary(t *testing.T) {
	m := sampleRelease()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(m) }))
	defer server.Close()
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "data"), 0700)
	library := filepath.Join(root, "data/library.json")
	os.WriteFile(library, []byte("preserve test library"), 0600)
	s := &Studio{root: root, client: server.Client()}
	if e := s.SetUpdateFeed(server.URL); e != nil {
		t.Fatal(e)
	}
	v, e := s.CheckForUpdates()
	if e != nil || v.Error != "" || !v.Available || v.DownloadURL != m.Assets[updatePlatform()].URL {
		t.Fatalf("%+v / %v", v, e)
	}
	m.VersionCode = appReleaseCode
	v, e = s.CheckForUpdates()
	if e != nil || v.Available {
		t.Fatal("current version treated as update")
	}
	b, _ := os.ReadFile(library)
	if string(b) != "preserve test library" {
		t.Fatal("update check changed user data")
	}
}
func TestUpdate404AndHTMLAreReportedWithoutOfferingDownload(t *testing.T) {
	for _, code := range []int{404, 200} {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(code)
			w.Write([]byte("<html>not a manifest</html>"))
		}))
		_, e := readRelease(context.Background(), server.Client(), server.URL)
		server.Close()
		if e == nil {
			t.Fatalf("HTTP %d accepted", code)
		}
	}
}
func TestClearingFeedDisablesChecksAndPersists(t *testing.T) {
	s := &Studio{root: t.TempDir(), client: &http.Client{}}
	if e := s.SetUpdateFeed(""); e != nil {
		t.Fatal(e)
	}
	other := &Studio{root: s.root, client: &http.Client{}}
	if other.GetUpdateStatus().Feed != "" {
		t.Fatal("disabled feed not preserved")
	}
	v, _ := other.CheckForUpdates()
	if v.Available || v.Error == "" {
		t.Fatal("disabled feed offered a download")
	}
}
