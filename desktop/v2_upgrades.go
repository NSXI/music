package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func qualityUpgrade(old, next Quality) bool {
	if next.Codec == "" {
		return false
	}
	if next.Lossless != old.Lossless {
		return next.Lossless
	}
	if next.Lossless {
		return next.Bits >= old.Bits && next.Rate >= old.Rate && (next.Bits > old.Bits || next.Rate > old.Rate)
	}
	return strings.EqualFold(old.Codec, next.Codec) && next.Bitrate > old.Bitrate
}
func (s *Studio) QueueTrackUpgrade(id, candidate string) (string, error) {
	t, ok := s.task(id)
	if !ok || t.Status != "done" {
		return "", errors.New("اختر أغنية محفوظة")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	song, ok := s.known[candidate]
	if !ok || !sameSong(t.Song, song) || song.Provider == "spotify" {
		return "", errors.New("افحص النتيجة أولًا؛ يلزم تطابق العنوان والفنان والمدة والنسخة")
	}
	for _, j := range s.state.Jobs {
		if j.UpgradeFor == id && j.Status != "cancelled" && j.Status != "failed" {
			return "", errors.New("توجد نسخة بديلة قيد التنزيل أو المراجعة")
		}
	}
	job := Task{ID: key(id + candidate + fmt.Sprint(time.Now().UnixNano())), Song: song, Status: "queued", UpgradeFor: id, Log: []string{"تنزيل بديل منفصل؛ الملف الأصلي محفوظ حتى الاعتماد"}}
	s.state.Jobs = append(s.state.Jobs, job)
	s.save()
	s.signal()
	return job.ID, nil
}

// The UI stops/unloads playback before calling this explicit reviewed action.
// Both original files remain on disk; song IDs and all user edits stay stable.
func (s *Studio) ApplyTrackUpgrade(upgradeID string) error {
	upgrade, ok := s.task(upgradeID)
	if !ok || upgrade.Status != "done" || upgrade.UpgradeFor == "" {
		return errors.New("انتظر اكتمال فحص النسخة البديلة")
	}
	old, ok := s.task(upgrade.UpgradeFor)
	if !ok || !qualityUpgrade(old.Quality, upgrade.Quality) {
		return errors.New("لم تثبت جودة ملف أعلى قابلة للمقارنة؛ احتُفظ بالأصل")
	}
	if _, e := os.Stat(upgrade.Path); e != nil {
		return errors.New("ملف النسخة البديلة غير موجود")
	}
	backup, _ := json.Marshal(old)
	if e := writeChanged(filepath.Join(s.root, "data", "audio-upgrades", old.ID+"-before-"+fmt.Sprint(time.Now().UnixMilli())+".json"), backup); e != nil {
		return e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := append([]Task{}, s.state.Jobs...)
	out := make([]Task, 0, len(previous)-1)
	for _, job := range previous {
		if job.ID == upgradeID {
			continue
		}
		if job.ID == old.ID {
			job.Path = upgrade.Path
			job.Hash = upgrade.Hash
			job.Quality = upgrade.Quality
			job.Bytes = upgrade.Bytes
			job.Total = upgrade.Total
			job.Source = upgrade.Source
			job.Local = false
			job.MetadataPending = true
			job.Log = append(job.Log, "اعتمدت نسخة أعلى جودة مع الاحتفاظ بالكلمات والتفضيلات")
		}
		out = append(out, job)
	}
	s.state.Jobs = out
	s.save()
	if s.state.StorageError != "" {
		s.state.Jobs = previous
		return errors.New(s.state.StorageError)
	}
	select {
	case s.metadataWake <- struct{}{}:
	default:
	}
	return nil
}
