package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (s *Studio) SendDeviceCommand(raw, action string, position float64) error {
	base, token, e := parseDeviceLink(raw)
	if e != nil {
		return e
	}
	c := DeviceCommand{Action: action, Position: position}
	if !validDeviceCommand(c) {
		return errors.New("أمر غير صالح")
	}
	b, _ := json.Marshal(c)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "POST", base+"/control", bytes.NewReader(b))
	req.Header.Set("X-Nawa-Link", token)
	req.Header.Set("Content-Type", "application/json")
	r, e := linkClient().Do(req)
	if e != nil {
		return e
	}
	defer r.Body.Close()
	if r.StatusCode != 204 {
		return fmt.Errorf("تعذر إرسال الأمر: HTTP %d", r.StatusCode)
	}
	return nil
}

type DeviceCommand struct {
	ID       string  `json:"id"`
	Action   string  `json:"action"`
	Position float64 `json:"position"`
}
type deviceLink struct {
	server   *http.Server
	token    string
	urls     []string
	expires  time.Time
	shared   map[string]bool
	commands []DeviceCommand
	playback map[string]any
}
type LinkTransfer struct {
	Running   bool   `json:"running"`
	Phase     string `json:"phase"`
	Completed int    `json:"completed"`
	Total     int    `json:"total"`
	Bytes     int64  `json:"bytes"`
	Error     string `json:"error"`
}
type DeviceLinkStatus struct {
	Running   bool         `json:"running"`
	URLs      []string     `json:"urls"`
	ExpiresAt int64        `json:"expiresAt"`
	Shared    int          `json:"shared"`
	Transfer  LinkTransfer `json:"transfer"`
}
type DeviceTrack struct {
	ID        string `json:"id"`
	Song      Song   `json:"song"`
	Bytes     int64  `json:"bytes"`
	Hash      string `json:"hash"`
	Extension string `json:"extension"`
	Cover     bool   `json:"cover"`
}
type DeviceManifest struct {
	Schema int             `json:"schema"`
	Name   string          `json:"name"`
	Tracks []DeviceTrack   `json:"tracks"`
	Backup WorkspaceBackup `json:"backup"`
}

func randomLinkToken() (string, error) {
	var b [24]byte
	if _, e := rand.Read(b[:]); e != nil {
		return "", e
	}
	return hex.EncodeToString(b[:]), nil
}
func (s *Studio) GetDeviceLinkStatus() DeviceLinkStatus {
	s.linkMu.Lock()
	defer s.linkMu.Unlock()
	out := DeviceLinkStatus{Transfer: s.linkTransfer}
	if s.link != nil && time.Now().Before(s.link.expires) {
		out.Running = true
		out.URLs = append([]string{}, s.link.urls...)
		out.ExpiresAt = s.link.expires.UnixMilli()
		out.Shared = len(s.link.shared)
	}
	return out
}
func (s *Studio) StartDeviceLink(ids []string) (DeviceLinkStatus, error) {
	if len(ids) > 10000 {
		return DeviceLinkStatus{}, errors.New("عدد مسارات كبير")
	}
	shared := map[string]bool{}
	for _, id := range ids {
		t, ok := s.task(id)
		if !ok || t.Status != "done" {
			return DeviceLinkStatus{}, errors.New("اختر مسارات محفوظة")
		}
		shared[id] = true
	}
	s.StopDeviceLink()
	listener, e := net.Listen("tcp4", "0.0.0.0:0")
	if e != nil {
		return DeviceLinkStatus{}, e
	}
	token, e := randomLinkToken()
	if e != nil {
		listener.Close()
		return DeviceLinkStatus{}, e
	}
	link := &deviceLink{token: token, expires: time.Now().Add(30 * time.Minute), shared: shared, playback: map[string]any{}}
	port := listener.Addr().(*net.TCPAddr).Port
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		ip, _, e := net.ParseCIDR(a.String())
		if e == nil && ip.To4() != nil && ip.IsPrivate() && !ip.IsLoopback() {
			link.urls = append(link.urls, fmt.Sprintf("http://%s:%d/#%s", ip.String(), port, token))
		}
	}
	if len(link.urls) == 0 {
		link.urls = []string{fmt.Sprintf("http://127.0.0.1:%d/#%s", port, token)}
	}
	link.server = &http.Server{ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { s.serveDeviceLink(link, w, r) })}
	s.linkMu.Lock()
	s.link = link
	s.linkMu.Unlock()
	go link.server.Serve(listener)
	go func() {
		timer := time.NewTimer(time.Until(link.expires))
		defer timer.Stop()
		<-timer.C
		s.linkMu.Lock()
		if s.link == link {
			s.link = nil
		}
		s.linkMu.Unlock()
		link.server.Close()
	}()
	return s.GetDeviceLinkStatus(), nil
}
func (s *Studio) StopDeviceLink() {
	s.linkMu.Lock()
	l := s.link
	s.link = nil
	s.linkMu.Unlock()
	if l != nil {
		l.server.Close()
	}
}
func (s *Studio) serveDeviceLink(link *deviceLink, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if time.Now().After(link.expires) {
		http.Error(w, "Pairing expired", 410)
		return
	}
	if r.URL.Path == "/" && r.Method == "GET" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, deviceRemotePage)
		return
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Nawa-Link")), []byte(link.token)) != 1 {
		http.Error(w, "Pair this device first", 403)
		return
	}
	if r.Method != "GET" && r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	switch {
	case r.URL.Path == "/manifest" && r.Method == "GET":
		b, e := s.ExportWorkspaceBackup()
		if e != nil {
			http.Error(w, "Backup unavailable", 500)
			return
		}
		manifest := DeviceManifest{Schema: 2, Name: "Nawa", Backup: b}
		for _, t := range s.GetState().Jobs {
			if !link.shared[t.ID] || t.Status != "done" {
				continue
			}
			st, e := os.Stat(t.Path)
			if e != nil {
				continue
			}
			manifest.Tracks = append(manifest.Tracks, DeviceTrack{ID: t.ID, Song: portableSong(t.Song), Bytes: st.Size(), Hash: t.Hash, Extension: strings.ToLower(filepath.Ext(t.Path)), Cover: t.Cover != ""})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(manifest)
	case (strings.HasPrefix(r.URL.Path, "/audio/") || strings.HasPrefix(r.URL.Path, "/cover/")) && r.Method == "GET":
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) != 2 || !link.shared[parts[1]] {
			http.NotFound(w, r)
			return
		}
		t, ok := s.task(parts[1])
		if !ok || t.Status != "done" {
			http.NotFound(w, r)
			return
		}
		path := t.Path
		if parts[0] == "cover" {
			path = t.Cover
		}
		if path == "" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, path)
	case r.URL.Path == "/control" && r.Method == "POST":
		var c DeviceCommand
		if json.NewDecoder(io.LimitReader(r.Body, 2048)).Decode(&c) != nil || !validDeviceCommand(c) {
			http.Error(w, "Invalid control", 400)
			return
		}
		c.ID, _ = randomLinkToken()
		s.linkMu.Lock()
		if len(link.commands) >= 32 {
			s.linkMu.Unlock()
			http.Error(w, "Control queue full", 429)
			return
		}
		link.commands = append(link.commands, c)
		s.linkMu.Unlock()
		w.WriteHeader(204)
	case r.URL.Path == "/state" && r.Method == "GET":
		s.linkMu.Lock()
		b, _ := json.Marshal(link.playback)
		s.linkMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	default:
		http.NotFound(w, r)
	}
}
func validDeviceCommand(c DeviceCommand) bool {
	switch c.Action {
	case "play", "pause", "next", "previous":
		return true
	case "seek":
		return finiteTime(c.Position) && c.Position < 24*3600
	}
	return false
}
func (s *Studio) PollDeviceCommands(title, artist string, position, duration float64, paused bool) []DeviceCommand {
	s.linkMu.Lock()
	defer s.linkMu.Unlock()
	if s.link == nil {
		return []DeviceCommand{}
	}
	s.link.playback = map[string]any{"title": title, "artist": artist, "position": position, "duration": duration, "paused": paused}
	commands := s.link.commands
	s.link.commands = nil
	return commands
}
func parseDeviceLink(raw string) (string, string, error) {
	u, e := url.Parse(strings.TrimSpace(raw))
	if e != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Path != "/" && u.Path != "" {
		return "", "", errors.New("أدخل رابط ربط نَوى المحلي")
	}
	ip := net.ParseIP(u.Hostname())
	if ip == nil || (!ip.IsPrivate() && !ip.IsLoopback()) {
		return "", "", errors.New("الربط متاح مع عنوان شبكة محلية فقط")
	}
	port, e := strconv.Atoi(u.Port())
	if e != nil || port < 1 || port > 65535 {
		return "", "", errors.New("منفذ ربط غير صالح")
	}
	token := u.Fragment
	if len(token) != 48 || strings.Trim(token, "0123456789abcdef") != "" {
		return "", "", errors.New("رمز الربط غير صالح")
	}
	u.Fragment = ""
	u.Path = ""
	return u.String(), token, nil
}
func linkClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("Link redirects are not allowed") }}
}
func linkGet(ctx context.Context, base, token, path string) (*http.Response, error) {
	req, e := http.NewRequestWithContext(ctx, "GET", base+path, nil)
	if e != nil {
		return nil, e
	}
	req.Header.Set("X-Nawa-Link", token)
	res, e := linkClient().Do(req)
	if e != nil {
		return nil, e
	}
	if res.StatusCode != 200 {
		res.Body.Close()
		return nil, fmt.Errorf("تعذر الربط: HTTP %d", res.StatusCode)
	}
	return res, nil
}
func (s *Studio) PreviewDeviceLibrary(raw string) (DeviceManifest, error) {
	base, token, e := parseDeviceLink(raw)
	if e != nil {
		return DeviceManifest{}, e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	r, e := linkGet(ctx, base, token, "/manifest")
	if e != nil {
		return DeviceManifest{}, e
	}
	defer r.Body.Close()
	var m DeviceManifest
	e = json.NewDecoder(io.LimitReader(r.Body, 64<<20)).Decode(&m)
	if e == nil && (m.Schema != 2 || len(m.Tracks) > 10000) {
		e = errors.New("بيانات جهاز غير متوافقة")
	}
	return m, e
}
func (s *Studio) StartDevicePull(raw string, ids []string, includeWorkspace bool) error {
	if _, _, e := parseDeviceLink(raw); e != nil {
		return e
	}
	if len(ids) > 1000 {
		return errors.New("انقل حتى 1000 مسار في الدفعة")
	}
	s.linkMu.Lock()
	if s.linkTransfer.Running {
		s.linkMu.Unlock()
		return errors.New("يوجد نقل قيد التنفيذ")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	s.linkCancel = cancel
	s.linkTransfer = LinkTransfer{Running: true, Phase: "الاتصال بالجهاز"}
	s.linkMu.Unlock()
	go func() {
		defer cancel()
		e := s.pullDeviceLibrary(ctx, raw, ids, includeWorkspace)
		s.linkMu.Lock()
		s.linkTransfer.Running = false
		s.linkCancel = nil
		if e != nil {
			s.linkTransfer.Error = e.Error()
			s.linkTransfer.Phase = "توقف النقل؛ الملفات المكتملة محفوظة"
		} else {
			s.linkTransfer.Phase = "اكتمل النقل"
		}
		s.linkMu.Unlock()
	}()
	return nil
}
func (s *Studio) CancelDeviceTransfer() {
	s.linkMu.Lock()
	defer s.linkMu.Unlock()
	if s.linkCancel != nil {
		s.linkCancel()
	}
}
func (s *Studio) pullDeviceLibrary(ctx context.Context, raw string, ids []string, includeWorkspace bool) error {
	base, token, e := parseDeviceLink(raw)
	if e != nil {
		return e
	}
	m, e := s.PreviewDeviceLibrary(raw)
	if e != nil {
		return e
	}
	wanted := map[string]bool{}
	for _, id := range ids {
		wanted[id] = true
	}
	selected := []DeviceTrack{}
	var total int64
	for _, t := range m.Tracks {
		if !wanted[t.ID] {
			continue
		}
		if t.ID == "" || strings.ContainsAny(t.ID, "/\\") || t.Bytes <= 0 || t.Bytes > 2<<30 || len(t.Hash) != 64 || strings.Trim(t.Hash, "0123456789abcdef") != "" {
			return errors.New("وصف ملف غير صالح")
		}
		switch t.Extension {
		case ".flac", ".mp3", ".m4a", ".ogg", ".opus", ".wav", ".aac", ".aiff", ".ape", ".wv":
		default:
			return errors.New("صيغة نقل غير مدعومة")
		}
		selected = append(selected, t)
		total += t.Bytes
	}
	if len(selected) != len(wanted) || total > 10<<30 {
		return errors.New("مسارات غير متاحة أو حجم الدفعة أكبر من 10 GB")
	}
	s.linkMu.Lock()
	s.linkTransfer.Total = len(selected)
	s.linkMu.Unlock()
	dir := filepath.Join(s.root, "data", "device-imports")
	if e = os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	for _, t := range selected {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		s.linkMu.Lock()
		s.linkTransfer.Phase = t.Song.Title
		s.linkMu.Unlock()
		dest := filepath.Join(dir, t.Hash+t.Extension)
		if fileHash(dest) != t.Hash {
			r, e := linkGet(ctx, base, token, "/audio/"+url.PathEscape(t.ID))
			if e != nil {
				return e
			}
			part := dest + ".part"
			f, e := os.OpenFile(part, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
			if e != nil {
				r.Body.Close()
				return e
			}
			n, e := io.Copy(f, io.LimitReader(r.Body, t.Bytes+1))
			ce := f.Close()
			r.Body.Close()
			if e == nil {
				e = ce
			}
			if e != nil || n != t.Bytes || fileHash(part) != t.Hash {
				os.Remove(part)
				return errors.New("تعذر التحقق من الملف المنقول؛ لم يُضف للمكتبة")
			}
			if e = os.Rename(part, dest); e != nil {
				return e
			}
		}
		q, _, e := s.probeDetails(ctx, dest)
		if e != nil {
			return errors.New("الملف المنقول لا يحتوي صوتًا قابلًا للقراءة")
		}
		if _, e = s.importPaths(ctx, []string{dest}); e != nil {
			return e
		}
		for _, local := range s.GetState().Jobs {
			if local.Hash == t.Hash {
				song := t.Song
				song.ID = local.Song.ID
				song.Provider = "local"
				song.Duration = q.Duration
				s.edit(local.ID, func(j *Task) { j.Song = song })
				if t.Cover && local.Cover == "" {
					if r, e := linkGet(ctx, base, token, "/cover/"+url.PathEscape(t.ID)); e == nil {
						b, e := io.ReadAll(io.LimitReader(r.Body, 8<<20+1))
						r.Body.Close()
						if e == nil && len(b) <= 8<<20 {
							if e = s.cacheDeviceCover(local.ID, b); e != nil {
								return e
							}
						}
					}
				}
				break
			}
		}
		s.linkMu.Lock()
		s.linkTransfer.Completed++
		s.linkTransfer.Bytes += t.Bytes
		s.linkMu.Unlock()
	}
	s.mu.Lock()
	s.save()
	s.mu.Unlock()
	if includeWorkspace {
		_, e = s.RestoreWorkspaceBackup(m.Backup, false)
	}
	return e
}
func (s *Studio) cacheDeviceCover(id string, b []byte) error {
	// Reuse the bounded artwork validator rather than trusting the peer's extension.
	if _, _, e := imageDecodeConfig(b); e != nil {
		return nil
	}
	p := filepath.Join(s.root, "data", "device-art", key(string(b))+".img")
	if e := writeChanged(p, b); e != nil {
		return e
	}
	s.edit(id, func(t *Task) { t.Cover = p })
	return nil
}
func imageDecodeConfig(b []byte) (int, int, error) {
	c, _, e := image.DecodeConfig(bytes.NewReader(b))
	if e != nil {
		return 0, 0, e
	}
	if c.Width <= 0 || c.Height <= 0 || int64(c.Width)*int64(c.Height) > 20_000_000 {
		return 0, 0, errors.New("غلاف أكبر من الحد")
	}
	return c.Width, c.Height, nil
}

const deviceRemotePage = `<!doctype html><html lang="ar" dir="rtl"><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>نَوى · جهاز التحكم</title><style>body{font:20px system-ui;background:#07090e;color:#f0f4fa;max-width:600px;margin:12vh auto;padding:24px}button,input{font:inherit;padding:16px;margin:8px;border-radius:14px;border:1px solid #54617a;background:#18233b;color:inherit}input{width:85%}small{color:#b9c9dc}</style><h1>نَوى</h1><h2 id="title">جهاز التحكم</h2><p id="artist"></p><p><button onclick="send('previous')">السابق</button><button onclick="send('play')">تشغيل</button><button onclick="send('pause')">إيقاف</button><button onclick="send('next')">التالي</button></p><input id="seek" aria-label="موضع التشغيل" type="range" min="0" max="1000" onchange="send('seek',Number(this.value)/1000*duration)"><p id="status" role="status"></p><small>أبقِ تطبيق نَوى ظاهرًا على الجهاز الآخر لاستقبال الأوامر.</small><script>const token=location.hash.slice(1);history.replaceState(null,'',location.pathname);let duration=0;async function send(action,position=0){try{const r=await fetch('/control',{method:'POST',headers:{'X-Nawa-Link':token,'Content-Type':'application/json'},body:JSON.stringify({action,position})});if(!r.ok)throw Error(r.status);document.getElementById('status').textContent='تم إرسال الأمر'}catch(e){document.getElementById('status').textContent='تعذر الاتصال أو انتهى الربط'}}async function poll(){try{const r=await fetch('/state',{headers:{'X-Nawa-Link':token}});if(!r.ok)throw Error();const s=await r.json();duration=s.duration||0;document.getElementById('title').textContent=s.title||'نَوى';document.getElementById('artist').textContent=s.artist||'';document.getElementById('seek').value=duration?s.position/duration*1000:0}catch(e){document.getElementById('status').textContent='الربط غير متاح'}}setInterval(poll,1500);poll();</script></html>`
