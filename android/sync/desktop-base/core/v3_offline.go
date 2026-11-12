package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type OfflineItem struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Artist      string  `json:"artist"`
	Quality     Quality `json:"quality"`
	Bytes       int64   `json:"bytes"`
	File        string  `json:"file,omitempty"`
	Cover       string  `json:"cover,omitempty"`
	Lyrics      string  `json:"lyrics,omitempty"`
	Translation string  `json:"translation,omitempty"`
	Error       string  `json:"error,omitempty"`
}

type OfflinePack struct {
	ID      string        `json:"id"`
	Name    string        `json:"name"`
	Created int64         `json:"created"`
	Bytes   int64         `json:"bytes"`
	Items   []OfflineItem `json:"items"`
}

type OfflineStatus struct {
	Running   bool   `json:"running"`
	Completed int    `json:"completed"`
	Total     int    `json:"total"`
	Error     string `json:"error,omitempty"`
}

type OfflineEstimate struct {
	AudioBytes int64         `json:"audioBytes"`
	UsedBytes  int64         `json:"usedBytes"`
	Items      []OfflineItem `json:"items"`
}

func (s *Studio) offlineRoot() string {
	return filepath.Join(s.root, "data", "offline-v3")
}

func validPackID(id string) bool {
	return len(id) == 48 && strings.Trim(id, "0123456789abcdef") == ""
}

func (s *Studio) ListOfflinePacks() []OfflinePack {
	out := []OfflinePack{}
	entries, _ := os.ReadDir(s.offlineRoot())
	for _, entry := range entries {
		if !entry.IsDir() || !validPackID(entry.Name()) {
			continue
		}
		file := filepath.Join(s.offlineRoot(), entry.Name(), "manifest.json")
		info, err := os.Stat(file)
		if err != nil || info.Size() > 4<<20 {
			continue
		}
		data, err := os.ReadFile(file)
		var pack OfflinePack
		if err == nil && json.Unmarshal(data, &pack) == nil && pack.ID == entry.Name() {
			out = append(out, pack)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created > out[j].Created })
	return out
}

func (s *Studio) EstimateOfflinePack(ids []string) (OfflineEstimate, error) {
	out := OfflineEstimate{Items: []OfflineItem{}}
	if len(ids) == 0 || len(ids) > 1000 {
		return out, errors.New("اختر من مسار إلى 1000 مسار")
	}
	for _, pack := range s.ListOfflinePacks() {
		out.UsedBytes += pack.Bytes
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		task, ok := s.task(id)
		item := OfflineItem{ID: id, Title: task.Song.Title, Artist: task.Song.Artist, Quality: task.Quality}
		if !ok || task.Status != "done" {
			item.Error = "المسار غير محفوظ"
		} else if info, err := os.Stat(task.Path); err != nil || !info.Mode().IsRegular() {
			item.Error = "ملف الصوت غير متاح"
		} else {
			item.Bytes = info.Size()
			out.AudioBytes += item.Bytes
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}

func (s *Studio) GetOfflineStatus() OfflineStatus {
	s.v3Mu.Lock()
	defer s.v3Mu.Unlock()
	return s.v3Offline
}

func (s *Studio) CancelOfflinePack() {
	s.v3Mu.Lock()
	defer s.v3Mu.Unlock()
	if s.v3OfflineCancel != nil {
		s.v3OfflineCancel()
	}
}

func (s *Studio) StartOfflinePack(name string, ids []string, quota int64, target, source string) error {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 400 || quota <= 0 || quota > 1<<40 {
		return errors.New("أدخل اسم الحزمة وحد المساحة")
	}
	s.v3Mu.Lock()
	defer s.v3Mu.Unlock()
	if s.v3Offline.Running {
		return errors.New("يجري تجهيز حزمة أخرى")
	}
	estimate, err := s.EstimateOfflinePack(ids)
	if err != nil {
		return err
	}
	if estimate.AudioBytes == 0 || estimate.AudioBytes > quota-estimate.UsedBytes {
		return errors.New("الحزمة تتجاوز المساحة المحددة أو لا تحتوي صوتًا محفوظًا")
	}
	id, err := randomLinkToken()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	s.v3OfflineCancel = cancel
	s.v3Offline = OfflineStatus{Running: true, Total: len(estimate.Items)}
	go func() {
		defer cancel()
		err := s.buildOfflinePack(ctx, OfflinePack{ID: id, Name: name, Created: time.Now().UnixMilli(), Items: estimate.Items}, quota-estimate.UsedBytes, target, source)
		s.v3Mu.Lock()
		defer s.v3Mu.Unlock()
		s.v3Offline.Running = false
		s.v3OfflineCancel = nil
		if err != nil {
			s.v3Offline.Error = err.Error()
		}
	}()
	return nil
}

func (s *Studio) buildOfflinePack(ctx context.Context, pack OfflinePack, available int64, target, source string) error {
	root := s.offlineRoot()
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(root, ".building-")
	if err != nil {
		return err
	}
	defer removeOwnedTemp(root, staging)
	var playlist strings.Builder
	playlist.WriteString("#EXTM3U\n")
	var pendingAudio int64
	for _, item := range pack.Items {
		if item.Error == "" {
			pendingAudio += item.Bytes
		}
	}
	for i := range pack.Items {
		if err := ctx.Err(); err != nil {
			return err
		}
		item := &pack.Items[i]
		if item.Error == "" {
			pendingAudio -= item.Bytes
			task, exists := s.task(item.ID)
			switch {
			case !exists:
				item.Error = "المسار لم يعد في المكتبة"
			case item.Bytes > available-pack.Bytes:
				item.Error = "المساحة المحددة لا تكفي"
			default:
				filename := fmt.Sprintf("%04d%s", i+1, safeOfflineExtension(task.Path))
				if err := copyOfflineFile(ctx, task.Path, filepath.Join(staging, filename), item.Bytes); err != nil {
					item.Error = "تعذر نسخ ملف الصوت"
				} else {
					item.File = filename
					pack.Bytes += item.Bytes
					fmt.Fprintf(&playlist, "#EXTINF:%d,%s\n%s\n", int(task.Song.Duration), strings.NewReplacer("\r", " ", "\n", " ").Replace(task.Song.Title), filename)
					s.writeOfflineExtras(ctx, staging, task, item, &pack, available-pendingAudio, target, source)
				}
			}
		}
		s.v3Mu.Lock()
		s.v3Offline.Completed++
		s.v3Mu.Unlock()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if pack.Bytes == 0 {
		return errors.New("تعذر حفظ أي مسار في الحزمة")
	}
	if err := os.WriteFile(filepath.Join(staging, "playlist.m3u8"), []byte(playlist.String()), 0600); err != nil {
		return err
	}
	raw, err := json.Marshal(pack)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(staging, "manifest.json"), raw, 0600); err != nil {
		return err
	}
	return os.Rename(staging, filepath.Join(root, pack.ID))
}

func safeOfflineExtension(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if len(ext) >= 2 && len(ext) <= 9 && strings.Trim(ext[1:], "abcdefghijklmnopqrstuvwxyz0123456789") == "" {
		return ext
	}
	return ".audio"
}

func copyOfflineFile(ctx context.Context, from, to string, length int64) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	buffer := make([]byte, 128<<10)
	var total int64
	for err == nil {
		if err = ctx.Err(); err != nil {
			break
		}
		n, readErr := in.Read(buffer)
		total += int64(n)
		if total > length {
			err = errors.New("ملف المصدر تغيّر أثناء النسخ")
			break
		}
		if n > 0 {
			_, err = out.Write(buffer[:n])
		}
		if err == nil && readErr != nil {
			if readErr != io.EOF || total != length {
				err = errors.New("النسخ غير مكتمل")
			}
			break
		}
	}
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(to)
	}
	return err
}

func (s *Studio) writeOfflineExtras(ctx context.Context, staging string, task Task, item *OfflineItem, pack *OfflinePack, quota int64, target, source string) {
	stem := strings.TrimSuffix(item.File, filepath.Ext(item.File))
	if info, err := os.Stat(task.Cover); err == nil && info.Size() <= min(8<<20, quota-pack.Bytes) {
		name := stem + safeOfflineExtension(task.Cover)
		if name == item.File {
			name = stem + "-cover" + safeOfflineExtension(task.Cover)
		}
		if copyOfflineFile(ctx, task.Cover, filepath.Join(staging, name), info.Size()) == nil {
			item.Cover = name
			pack.Bytes += info.Size()
		}
	}
	write := func(name string, data []byte) string {
		if len(data) == 0 || int64(len(data)) > quota-pack.Bytes {
			return ""
		}
		if os.WriteFile(filepath.Join(staging, name), data, 0600) != nil {
			return ""
		}
		pack.Bytes += int64(len(data))
		return name
	}
	if cache, err := s.readLyricCache(task.Song); err == nil {
		raw, _ := json.Marshal(cache)
		item.Lyrics = write(stem+"-lyrics.json", raw)
		if item.Lyrics != "" && validTranslationLanguage(target, source) {
			if rows, err := s.translationOriginal(task); err == nil {
				if v, err := s.readTranslation(task, rows, target, source); err == nil && v.Exists {
					raw, _ := json.Marshal(v)
					item.Translation = write(stem+"-translation.json", raw)
				}
			}
		}
	}
}

func (s *Studio) DeleteOfflinePack(id string) error {
	if !validPackID(id) {
		return errors.New("حزمة غير صالحة")
	}
	s.v3Mu.Lock()
	defer s.v3Mu.Unlock()
	if s.v3Offline.Running {
		return errors.New("انتظر تجهيز الحزمة الحالية")
	}
	root := s.offlineRoot()
	removeOwnedTemp(root, filepath.Join(root, id))
	return nil
}

func (s *Studio) audioForPlayback(task Task) string {
	if _, err := os.Stat(task.Path); err == nil {
		return task.Path
	}
	for _, pack := range s.ListOfflinePacks() {
		for _, item := range pack.Items {
			if item.ID != task.ID || item.File == "" || filepath.Base(item.File) != item.File {
				continue
			}
			path := filepath.Join(s.offlineRoot(), pack.ID, item.File)
			if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() && info.Size() == item.Bytes {
				return path
			}
		}
	}
	return task.Path
}
