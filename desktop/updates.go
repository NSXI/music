package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	wr "github.com/wailsapp/wails/v2/pkg/runtime"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

type UpdateAsset struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}
type UpdateManifest struct {
	Schema         int                    `json:"schema"`
	AppID          string                 `json:"appId"`
	Version        string                 `json:"version"`
	VersionCode    int                    `json:"versionCode"`
	SharedRevision string                 `json:"sharedRevision"`
	ReleaseURL     string                 `json:"releaseUrl"`
	Notes          string                 `json:"notes"`
	Assets         map[string]UpdateAsset `json:"assets"`
	Components     map[string]UpdateAsset `json:"components,omitempty"`
}
type UpdateView struct {
	Feed           string `json:"feed"`
	CurrentVersion string `json:"currentVersion"`
	CurrentCode    int    `json:"currentCode"`
	Platform       string `json:"platform"`
	Checking       bool   `json:"checking"`
	Available      bool   `json:"available"`
	Version        string `json:"version"`
	VersionCode    int    `json:"versionCode"`
	DownloadURL    string `json:"downloadUrl"`
	ReleaseURL     string `json:"releaseUrl"`
	SHA256         string `json:"sha256"`
	Bytes          int64  `json:"bytes"`
	Notes          string `json:"notes"`
	CheckedAt      string `json:"checkedAt"`
	Error          string `json:"error"`
	SharedRevision string `json:"sharedRevision"`
}
type updateSession struct {
	sync.Mutex
	view       UpdateView
	generation uint64
	loaded     bool
}

var updateSessions sync.Map

func updatePlatform() string {
	if runtime.GOOS == "android" {
		return "android-arm64"
	}
	return "windows-x64"
}
func safeUpdateURL(address string) bool {
	u, e := url.Parse(address)
	return e == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.Fragment == ""
}
func validateUpdateManifest(m UpdateManifest) error {
	if m.Schema != 1 || m.AppID != "dev.nawa.music" || m.VersionCode < 1 || m.VersionCode > 2147483647 || len(m.Version) > 80 || m.Version == "" {
		return errors.New("بيانات إصدار غير متوافقة / Invalid release identity")
	}
	hash := regexp.MustCompile(`^[a-fA-F0-9]{64}$`)
	if !hash.MatchString(m.SharedRevision) || !safeUpdateURL(m.ReleaseURL) || len(m.Notes) > 16000 {
		return errors.New("بيانات إصدار ناقصة / Incomplete release manifest")
	}
	for _, platform := range []string{"windows-x64", "android-arm64"} {
		a, ok := m.Assets[platform]
		if !ok || !safeUpdateURL(a.URL) || !hash.MatchString(a.SHA256) || a.Bytes <= 0 {
			return fmt.Errorf("ملف %s أو بصمته غير موجود / Missing platform asset", platform)
		}
	}
	return nil
}
func (s *Studio) updates() *updateSession {
	raw, _ := updateSessions.LoadOrStore(s, &updateSession{})
	u := raw.(*updateSession)
	u.Lock()
	defer u.Unlock()
	if !u.loaded {
		u.loaded = true
		feed := defaultUpdateFeed
		var saved struct {
			Feed string `json:"feed"`
		}
		if b, e := os.ReadFile(filepath.Join(s.root, "data", "updates.json")); e == nil && json.Unmarshal(b, &saved) == nil && (saved.Feed == "" || safeUpdateURL(saved.Feed)) {
			feed = saved.Feed
		}
		u.view = UpdateView{Feed: feed, CurrentVersion: appReleaseVersion, CurrentCode: appReleaseCode, Platform: updatePlatform()}
	}
	return u
}
func (s *Studio) GetUpdateStatus() UpdateView {
	u := s.updates()
	u.Lock()
	defer u.Unlock()
	return u.view
}
func (s *Studio) SetUpdateFeed(feed string) error {
	feed = strings.TrimSpace(feed)
	if feed != "" && !safeUpdateURL(feed) {
		return errors.New("استخدم رابط HTTPS صالحًا / Use a valid HTTPS URL")
	}
	if len(feed) > 2048 {
		return errors.New("رابط طويل جدًا / URL too long")
	}
	b, _ := json.Marshal(map[string]string{"feed": feed})
	os.MkdirAll(filepath.Join(s.root, "data"), 0700)
	if e := writeChanged(filepath.Join(s.root, "data", "updates.json"), b); e != nil {
		return e
	}
	u := s.updates()
	u.Lock()
	defer u.Unlock()
	u.generation++
	u.view = UpdateView{Feed: feed, CurrentVersion: appReleaseVersion, CurrentCode: appReleaseCode, Platform: updatePlatform()}
	return nil
}
func readRelease(ctx context.Context, client *http.Client, feed string) (UpdateManifest, error) {
	var manifest UpdateManifest
	if !safeUpdateURL(feed) {
		return manifest, errors.New("أضف رابط تحديث HTTPS أولًا / Configure an HTTPS feed")
	}
	request, e := http.NewRequestWithContext(ctx, "GET", feed, nil)
	if e != nil {
		return manifest, e
	}
	request.Header.Set("User-Agent", "Nawa/"+appReleaseVersion)
	request.Header.Set("Accept", "application/json")
	safe := *client
	safe.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if len(via) > 5 || !safeUpdateURL(r.URL.String()) {
			return errors.New("Unsafe update redirect")
		}
		return nil
	}
	response, e := safe.Do(request)
	if e != nil {
		return manifest, errors.New("تعذر الاتصال؛ جرّب لاحقًا / Update server unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode == 404 {
		return manifest, errors.New("لم يُنشر ملف تحديث عام بعد / No public update manifest published yet")
	}
	if response.StatusCode != 200 {
		return manifest, fmt.Errorf("خادم التحديث: HTTP %d", response.StatusCode)
	}
	data, e := io.ReadAll(io.LimitReader(response.Body, 262145))
	if e != nil || len(data) > 262144 {
		return manifest, errors.New("ملف إصدار غير صالح أو كبير جدًا / Invalid manifest size")
	}
	if e = json.Unmarshal(data, &manifest); e != nil {
		return manifest, errors.New("ملف التحديث ليس JSON صالحًا / Invalid update JSON")
	}
	return manifest, validateUpdateManifest(manifest)
}
func (s *Studio) CheckForUpdates() (UpdateView, error) {
	u := s.updates()
	u.Lock()
	if u.view.Checking {
		v := u.view
		u.Unlock()
		return v, nil
	}
	feed, generation := u.view.Feed, u.generation
	u.view.Checking = true
	u.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 18*time.Second)
	defer cancel()
	m, e := readRelease(ctx, s.client, feed)
	u.Lock()
	defer u.Unlock()
	if generation != u.generation {
		return u.view, nil
	}
	v := UpdateView{Feed: feed, CurrentVersion: appReleaseVersion, CurrentCode: appReleaseCode, Platform: updatePlatform(), CheckedAt: time.Now().UTC().Format(time.RFC3339)}
	if e != nil {
		v.Error = e.Error()
		u.view = v
		return v, nil
	}
	a := m.Assets[v.Platform]
	v.Available = m.VersionCode > appReleaseCode
	v.Version = m.Version
	v.VersionCode = m.VersionCode
	v.ReleaseURL = m.ReleaseURL
	v.DownloadURL = a.URL
	v.SHA256 = a.SHA256
	v.Bytes = a.Bytes
	v.Notes = m.Notes
	v.SharedRevision = m.SharedRevision
	u.view = v
	return v, nil
}
func (s *Studio) OpenUpdatePage() error {
	v := s.GetUpdateStatus()
	if !v.Available || !safeUpdateURL(v.DownloadURL) {
		return errors.New("لا يوجد إصدار جديد متحقق منه / No verified update available")
	}
	wr.BrowserOpenURL(s.ctx, v.DownloadURL)
	return nil
}
