package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type cloudConfig struct {
	Endpoint string `json:"endpoint"`
	Username string `json:"username"`
	Password string `json:"password"`
	ETag     string `json:"etag"`
}
type CloudSyncView struct {
	Configured bool   `json:"configured"`
	Endpoint   string `json:"endpoint"`
	Username   string `json:"username"`
}

func (s *Studio) readCloudConfig() (cloudConfig, error) {
	p := filepath.Join(s.root, "data", "cloud-sync.sealed")
	b, e := os.ReadFile(p)
	if e != nil {
		return cloudConfig{}, e
	}
	b, e = protectSecret(b, true)
	if e != nil {
		return cloudConfig{}, e
	}
	var c cloudConfig
	e = json.Unmarshal(b, &c)
	return c, e
}
func (s *Studio) writeCloudConfig(c cloudConfig) error {
	b, e := json.Marshal(c)
	if e != nil {
		return e
	}
	b, e = protectSecret(b, false)
	if e != nil {
		return errors.New("تعذر حفظ إعدادات المزامنة مشفرة")
	}
	return writeChanged(filepath.Join(s.root, "data", "cloud-sync.sealed"), b)
}
func (s *Studio) GetCloudSyncConfig() CloudSyncView {
	c, e := s.readCloudConfig()
	return CloudSyncView{Configured: e == nil && c.Endpoint != "", Endpoint: c.Endpoint, Username: c.Username}
}
func (s *Studio) SetCloudSyncConfig(endpoint, username, password string) error {
	if endpoint == "" {
		e := os.Remove(filepath.Join(s.root, "data", "cloud-sync.sealed"))
		if os.IsNotExist(e) {
			return nil
		}
		return e
	}
	u, e := url.Parse(endpoint)
	if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.RawQuery != "" || !strings.HasSuffix(strings.ToLower(u.Path), ".json") || len(endpoint) > 2000 || len(username) > 1000 || len(password) > 4096 {
		return errors.New("استخدم رابط HTTPS كاملًا لملف JSON على خادم WebDAV")
	}
	if password == "" {
		if old, e := s.readCloudConfig(); e == nil && old.Endpoint == endpoint && old.Username == username {
			password = old.Password
		}
	}
	return s.writeCloudConfig(cloudConfig{Endpoint: endpoint, Username: username, Password: password})
}
func (s *Studio) cloudRequest(ctx context.Context, method string, c cloudConfig, body []byte) (*http.Response, error) {
	req, e := http.NewRequestWithContext(ctx, method, c.Endpoint, bytes.NewReader(body))
	if e != nil {
		return nil, e
	}
	req.SetBasicAuth(c.Username, c.Password)
	req.Header.Set("Content-Type", "application/json")
	if method == "PUT" {
		if c.ETag != "" {
			req.Header.Set("If-Match", c.ETag)
		} else {
			req.Header.Set("If-None-Match", "*")
		}
	}
	client := &http.Client{Timeout: 2 * time.Minute, Transport: s.client.Transport, CheckRedirect: func(*http.Request, []*http.Request) error {
		return errors.New("تغيّر عنوان المزامنة؛ حدّثه من الإعدادات")
	}}
	return client.Do(req)
}
func (s *Studio) UploadCloudBackup() error {
	c, e := s.readCloudConfig()
	if e != nil {
		return errors.New("أعدّ خادم WebDAV أولًا")
	}
	backup, e := s.ExportWorkspaceBackup()
	if e != nil {
		return e
	}
	b, e := json.Marshal(backup)
	if e != nil {
		return e
	}
	if len(b) > 64<<20 {
		return errors.New("النسخة أكبر من حد المزامنة 64 MB")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	r, e := s.cloudRequest(ctx, "PUT", c, b)
	if e != nil {
		return errors.New("تعذر الاتصال بخادم المزامنة")
	}
	defer r.Body.Close()
	if r.StatusCode == 412 {
		return errors.New("توجد نسخة سحابية أحدث أو غير مراجعة؛ نزّلها للمراجعة قبل الاستبدال")
	}
	if r.StatusCode < 200 || r.StatusCode >= 300 {
		return errors.New("الخادم رفض رفع النسخة؛ راجع الصلاحيات والرابط")
	}
	c.ETag = r.Header.Get("ETag")
	return s.writeCloudConfig(c)
}
func (s *Studio) DownloadCloudBackup() (WorkspaceBackup, error) {
	c, e := s.readCloudConfig()
	if e != nil {
		return WorkspaceBackup{}, errors.New("أعدّ خادم WebDAV أولًا")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	r, e := s.cloudRequest(ctx, "GET", c, nil)
	if e != nil {
		return WorkspaceBackup{}, errors.New("تعذر الاتصال بخادم المزامنة")
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return WorkspaceBackup{}, errors.New("تعذر قراءة النسخة من الخادم")
	}
	b, e := io.ReadAll(io.LimitReader(r.Body, (64<<20)+1))
	if e != nil || len(b) > 64<<20 {
		return WorkspaceBackup{}, errors.New("حجم النسخة غير صالح")
	}
	var backup WorkspaceBackup
	if e = json.Unmarshal(b, &backup); e != nil || backup.Schema != 2 {
		return backup, errors.New("ملف المزامنة غير صالح")
	}
	c.ETag = r.Header.Get("ETag")
	if e = s.writeCloudConfig(c); e != nil {
		return backup, e
	}
	return backup, nil
}
