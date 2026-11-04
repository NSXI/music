package main

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

type ComponentState struct {
	ID        string `json:"id"`
	Ready     bool   `json:"ready"`
	Supported bool   `json:"supported"`
	Running   bool   `json:"running"`
	Bytes     int64  `json:"bytes"`
	Total     int64  `json:"total"`
	Phase     string `json:"phase"`
	Error     string `json:"error"`
}
type componentSession struct {
	sync.Mutex
	state  ComponentState
	cancel context.CancelFunc
}

var componentSessions sync.Map

func (s *Studio) componentSession() *componentSession {
	v, _ := componentSessions.LoadOrStore(s, &componentSession{})
	return v.(*componentSession)
}
func (s *Studio) GetOptionalComponents() []ComponentState {
	session := s.componentSession()
	session.Lock()
	active := session.state
	session.Unlock()
	out := []ComponentState{}
	for _, id := range []string{"windows-alignment", "windows-translation"} {
		v := ComponentState{ID: id, Supported: runtime.GOOS == "windows"}
		if active.ID == id {
			v = active
			v.Supported = runtime.GOOS == "windows"
		}
		if id == "windows-alignment" {
			v.Ready = s.alignmentReady()
		} else {
			v.Ready = s.translationReady()
		}
		out = append(out, v)
	}
	return out
}
func (s *Studio) CancelComponentDownload() {
	session := s.componentSession()
	session.Lock()
	defer session.Unlock()
	if session.cancel != nil {
		session.cancel()
	}
}
func (s *Studio) StartComponentDownload(id string) error {
	if runtime.GOOS != "windows" || id != "windows-alignment" && id != "windows-translation" {
		return errors.New("استخدم إعداد نماذج أندرويد لهذا الجهاز")
	}
	session := s.componentSession()
	session.Lock()
	if session.state.Running {
		session.Unlock()
		return errors.New("يوجد مكون قيد التنزيل")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	session.cancel = cancel
	session.state = ComponentState{ID: id, Running: true, Supported: true, Phase: "قراءة الإصدار المتوافق"}
	session.Unlock()
	go func() {
		defer cancel()
		e := s.downloadComponent(ctx, id)
		session.Lock()
		defer session.Unlock()
		session.state.Running = false
		session.cancel = nil
		if e != nil {
			session.state.Error = e.Error()
			session.state.Phase = "لم يكتمل تثبيت المكون"
		} else {
			session.state.Phase = "جاهز للاستخدام"
		}
	}()
	return nil
}
func (s *Studio) downloadComponent(ctx context.Context, id string) error {
	m, e := readRelease(ctx, s.client, s.GetUpdateStatus().Feed)
	if e != nil {
		return e
	}
	if m.VersionCode != appReleaseCode {
		return errors.New("حدّث التطبيق للإصدار المنشور قبل تنزيل مكوناته")
	}
	asset, ok := m.Components[id]
	if !ok || !safeUpdateURL(asset.URL) || len(asset.SHA256) != 64 || strings.Trim(strings.ToLower(asset.SHA256), "0123456789abcdef") != "" || asset.Bytes <= 0 || asset.Bytes > 2<<30 {
		return errors.New("لا توجد حزمة مكونات متوافقة متاحة في الإصدار")
	}
	dir := filepath.Join(s.root, "data", "components")
	if e = os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	archive := filepath.Join(dir, id+".zip.partial")
	defer os.Remove(archive)
	req, _ := http.NewRequestWithContext(ctx, "GET", asset.URL, nil)
	client := *s.client
	client.Timeout = 0
	client.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if len(via) > 5 || !safeUpdateURL(r.URL.String()) {
			return errors.New("رابط تنزيل غير آمن")
		}
		return nil
	}
	res, e := client.Do(req)
	if e != nil {
		return errors.New("تعذر تنزيل المكون")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("تنزيل المكون: HTTP %d", res.StatusCode)
	}
	f, e := os.OpenFile(archive, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	hash := sha256.New()
	session := s.componentSession()
	session.Lock()
	session.state.Total = asset.Bytes
	session.state.Phase = "تنزيل المكون الاختياري"
	session.Unlock()
	buffer := make([]byte, 128<<10)
	var total int64
	for {
		n, readErr := res.Body.Read(buffer)
		if n > 0 {
			total += int64(n)
			if total > asset.Bytes {
				f.Close()
				return errors.New("حجم المكون لا يطابق الإصدار")
			}
			if _, e = f.Write(buffer[:n]); e != nil {
				f.Close()
				return e
			}
			hash.Write(buffer[:n])
			session.Lock()
			session.state.Bytes = total
			session.Unlock()
		}
		if readErr != nil {
			if readErr != io.EOF {
				f.Close()
				return readErr
			}
			break
		}
	}
	if e = f.Close(); e != nil {
		return e
	}
	if total != asset.Bytes || hex.EncodeToString(hash.Sum(nil)) != strings.ToLower(asset.SHA256) {
		return errors.New("بصمة المكون لا تطابق الإصدار؛ لم يُثبّت")
	}
	release, e := s.acquireHeavyTask(ctx, "component-install")
	if e != nil {
		return e
	}
	defer release()
	session.Lock()
	session.state.Phase = "التحقق وتثبيت المكون"
	session.Unlock()
	return installComponentArchive(ctx, s.root, id, archive)
}
func componentMemberAllowed(id, name string) bool {
	if strings.Contains(name, "\\") || strings.HasPrefix(name, "/") || strings.Contains(name, ":") {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(name))
	if clean != strings.TrimSuffix(name, "/") || strings.HasPrefix(clean, "../") {
		return false
	}
	if id == "windows-alignment" {
		return clean == "tools/models/base.pt" || strings.HasPrefix(clean, "tools/aligner/")
	}
	if id == "windows-translation" {
		return strings.HasPrefix(clean, "tools/translator/") || strings.HasPrefix(clean, "tools/models/nllb-int8/")
	}
	return false
}
func installComponentArchive(ctx context.Context, root, id, archive string) error {
	z, e := zip.OpenReader(archive)
	if e != nil {
		return e
	}
	defer z.Close()
	if len(z.File) > 30000 {
		return errors.New("عدد ملفات المكون أكبر من الحد")
	}
	var uncompressed uint64
	seen := map[string]bool{}
	for _, f := range z.File {
		if f.FileInfo().IsDir() {
			continue
		}
		lower := strings.ToLower(f.Name)
		if !componentMemberAllowed(id, f.Name) || f.Mode()&os.ModeSymlink != 0 || seen[lower] {
			return errors.New("مسار غير مسموح داخل حزمة المكون")
		}
		seen[lower] = true
		if f.UncompressedSize64 > 8<<30 {
			return errors.New("ملف مكون أكبر من الحد")
		}
		uncompressed += f.UncompressedSize64
		if uncompressed > 8<<30 {
			return errors.New("حجم تثبيت المكون أكبر من الحد")
		}
	}
	stage, e := os.MkdirTemp(filepath.Join(root, "data", "components"), "install-")
	if e != nil {
		return e
	}
	defer removeOwnedTemp(filepath.Join(root, "data", "components"), stage)
	files := []string{}
	for _, f := range z.File {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if f.FileInfo().IsDir() {
			continue
		}
		dest := filepath.Join(stage, "new", filepath.FromSlash(f.Name))
		if e = os.MkdirAll(filepath.Dir(dest), 0700); e != nil {
			return e
		}
		input, e := f.Open()
		if e != nil {
			return e
		}
		output, e := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			input.Close()
			return e
		}
		n, e := io.Copy(output, io.LimitReader(input, int64(f.UncompressedSize64)+1))
		ce := output.Close()
		input.Close()
		if e == nil {
			e = ce
		}
		if e != nil || uint64(n) != f.UncompressedSize64 {
			return errors.New("ملف مكون تالف")
		}
		files = append(files, f.Name)
	}
	worker := "tools/aligner/MadarAlignV2.exe"
	if id == "windows-translation" {
		worker = "tools/translator/MadarTranslate.exe"
	}
	if !seen[strings.ToLower(worker)] {
		return errors.New("حزمة المكون لا تحتوي محرك التشغيل")
	}
	if id == "windows-alignment" && !seen["tools/models/base.pt"] || id == "windows-translation" && (!seen["tools/models/nllb-int8/model.bin"] || !seen["tools/models/nllb-int8/tokenizer.json"]) {
		return errors.New("حزمة المكون لا تحتوي النموذج كاملًا")
	}
	// Install the entry executable last. Roll back overwritten files if any copy
	// fails, including a DLL locked by an older app; never terminate that process.
	sort.Slice(files, func(i, j int) bool {
		if files[i] == worker {
			return false
		}
		if files[j] == worker {
			return true
		}
		return files[i] < files[j]
	})
	installed := []string{}
	existed := map[string]bool{}
	rollback := func() {
		for i := len(installed) - 1; i >= 0; i-- {
			name := installed[i]
			dest := filepath.Join(root, filepath.FromSlash(name))
			if existed[name] {
				copyAtomic(filepath.Join(stage, "old", filepath.FromSlash(name)), dest)
			} else {
				os.Remove(dest)
			}
		}
	}
	for _, name := range files {
		if ctx.Err() != nil {
			rollback()
			return ctx.Err()
		}
		dest := filepath.Join(root, filepath.FromSlash(name))
		if st, e := os.Lstat(dest); e == nil {
			if st.Mode()&os.ModeSymlink != 0 {
				rollback()
				return errors.New("مسار تثبيت غير صالح")
			}
			old := filepath.Join(stage, "old", filepath.FromSlash(name))
			os.MkdirAll(filepath.Dir(old), 0700)
			if e = copyAtomic(dest, old); e != nil {
				rollback()
				return e
			}
			existed[name] = true
		}
		if e = os.MkdirAll(filepath.Dir(dest), 0700); e != nil {
			rollback()
			return e
		}
		if e = copyAtomic(filepath.Join(stage, "new", filepath.FromSlash(name)), dest); e != nil {
			rollback()
			return errors.New("تعذر تثبيت ملفات المكون؛ أغلق محرك المعالجة القديم ثم أعد المحاولة")
		}
		installed = append(installed, name)
	}
	return nil
}
