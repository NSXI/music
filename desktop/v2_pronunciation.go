package main

import (
	"errors"
	"strings"
)

func (s *Studio) GetPronunciation(id string) ([]string, error) {
	t, ok := s.task(id)
	if !ok {
		return nil, errors.New("المسار غير موجود")
	}
	c, e := s.readLyricCache(t.Song)
	if e != nil {
		return []string{}, nil
	}
	return append([]string{}, c.Pronunciation...), nil
}

// Pronunciation is optional user-supplied text, never machine word timestamps.
func (s *Studio) SavePronunciation(id, revision string, rows []string) error {
	t, ok := s.task(id)
	if !ok {
		return errors.New("المسار غير موجود")
	}
	s.translationMu.Lock()
	defer s.translationMu.Unlock()
	c, e := s.readLyricCache(t.Song)
	if e != nil {
		return e
	}
	if lyricEditRevision(c) != revision {
		return errors.New("تغيّرت الكلمات؛ أعد فتح المحرر")
	}
	original := strings.Split(strings.TrimSpace(stripLyricText(lyricRecordText(c))), "\n")
	if len(rows) != len(original) {
		return errors.New("عدد سطور النطق لا يطابق الكلمات")
	}
	total := 0
	for _, r := range rows {
		total += len(r)
	}
	if total > 2<<20 {
		return errors.New("النص أكبر من الحد")
	}
	if e = s.backupLyricEdit(t, c); e != nil {
		return e
	}
	c.Pronunciation = rows
	return s.writeLyricCache(c)
}
