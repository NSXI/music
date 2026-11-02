package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func writeChanged(path string, data []byte) error {
	if old, e := os.ReadFile(path); e == nil && bytes.Equal(old, data) {
		return nil
	}
	dir := filepath.Dir(path)
	if e := os.MkdirAll(dir, 0755); e != nil {
		return e
	}
	f, e := os.CreateTemp(dir, ".madar-text-*.tmp")
	if e != nil {
		return e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, e = f.Write(data); e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	return os.Rename(tmp, path)
}
func (s *Studio) SaveLyricsText(id, text string, timed bool) error {
	s.translationMu.Lock()
	defer s.translationMu.Unlock()
	t, ok := s.task(id)
	if !ok || t.Path == "" {
		return errors.New("اختر ملفًا صوتيًا محفوظًا")
	}
	if len(text) > 2<<20 || strings.TrimSpace(text) == "" {
		return errors.New("أدخل كلمات غير فارغة بحجم أقل من 2 MB")
	}
	if timed && !validLRC(text, t.Song.Duration) {
		return errors.New("ملف LRC لا يحتوي توقيت سطور صالحًا لهذا التسجيل")
	}
	record := LyricsRecord{Title: t.Song.Title, Artist: t.Song.Artist, Album: t.Song.Album, Duration: t.Song.Duration, Provider: "local"}
	if timed {
		record.Synced = text
	} else {
		record.Plain = text
	}
	if e := s.writeLyricCache(LyricCache{Song: t.Song, Record: record}); e != nil {
		return e
	}
	_, e := s.persistLyricView(t, record)
	return e
}
func (s *Studio) autoAlign(id string) {
	if !s.alignmentReady() {
		return
	}
	candidate := filepath.Join(s.root, "data", "alignment", id+"-result.json")
	if raw, e := os.ReadFile(candidate); e == nil {
		var previous alignmentOutput
		if json.Unmarshal(raw, &previous) == nil && (strings.Contains(previous.Report.Model,"Android Whisper") || previous.Report.Refined || strings.Contains(previous.Report.Model, "word refinement v2")) {
			if task, ok := s.task(id); ok {
				original := ""
				if cache, e := s.readLyricCache(task.Song); e == nil {
					original = cache.Record.Plain
					if cache.Record.Synced != "" {
						original = cache.Record.Synced
					}
				} else if task.Lyrics != "" {
					if b, e := os.ReadFile(task.Lyrics); e == nil {
						original = string(b)
					}
				}
				saved := previous.Original
				if saved == "" {
					saved = previous.LRC
				}
				if original != "" && alignmentIdentity(stripLyricText(original)) == alignmentIdentity(stripLyricText(saved)) && validateAlignment(previous, task.Song.Duration) == nil {
					return
				}
			}
		}
	}
	s.mu.Lock()
	busy := s.alignState.Running
	same := s.alignState.TaskID == id && busy
	s.mu.Unlock()
	if same {
		return
	}
	if busy {
		s.queueAlignment(id)
		return
	}
	if e := s.StartAlignment(id, s.GetState().Preferences.AlignmentLanguage); e != nil {
		s.queueAlignment(id)
	}
}
