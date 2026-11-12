package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

func (s *Studio) StartFullLyricReview(id, language string) error {
	task, ok := s.task(id)
	if !ok || task.Status != "done" || !finiteTime(task.Song.Duration) || task.Song.Duration <= 0 || task.Song.Duration > 1200 {
		return errors.New("اختر أغنية محفوظة حتى 20 دقيقة")
	}
	if !s.alignmentReady() {
		return errors.New(s.missingAlignmentMessage())
	}
	doc, err := s.GetLyricDocument(id)
	if err != nil {
		return err
	}
	text := stripLyricText(doc.Text)
	if len([]rune(text)) < 8 {
		return errors.New("أضف كلمات الأغنية قبل تحليلها")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.alignState.Running {
		return errors.New("يوجد تحليل قيد التنفيذ")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	s.alignCancel = cancel
	s.alignState = AlignmentState{Running: true, Ready: true, Range: true, TaskID: id, Phase: "تحليل الأغنية كاملة للمراجعة"}
	go func() {
		defer cancel()
		err := s.runRangeAlignment(ctx, task, doc.Revision, 0, task.Song.Duration, text, language, true)
		s.mu.Lock()
		s.alignState.Running = false
		s.alignCancel = nil
		if err != nil {
			s.alignState.Error = err.Error()
			s.alignState.Phase = "لم يتغير توقيت الكلمات"
		}
		s.mu.Unlock()
		s.nextAlignment()
	}()
	return nil
}

func (s *Studio) exportSRT(id string, doc LyricEditDocument) (string, error) {
	task, ok := s.task(id)
	if !ok || !doc.Timed {
		return "", errors.New("يحتاج SRT إلى توقيت سطور أو كلمات")
	}
	lines := doc.Lines
	if len(lines) == 0 {
		rows, err := s.translationOriginal(task)
		if err != nil {
			return "", err
		}
		for i, row := range rows {
			if row.Time == nil {
				continue
			}
			end := task.Song.Duration
			if i+1 < len(rows) && rows[i+1].Time != nil {
				end = *rows[i+1].Time
			}
			lines = append(lines, WordLine{Start: max(0, *row.Time), End: end, Text: row.Original})
		}
	}
	var out strings.Builder
	number := 0
	for _, line := range lines {
		if !finiteTime(line.Start) || !finiteTime(line.End) || line.End <= line.Start {
			continue
		}
		number++
		fmt.Fprintf(&out, "%d\n%s --> %s\n%s\n\n", number, srtTimestamp(line.Start), srtTimestamp(line.End), strings.TrimSpace(line.Text))
	}
	if number == 0 {
		return "", errors.New("ما فيه سطور صالحة للتصدير")
	}
	return out.String(), nil
}

func srtTimestamp(seconds float64) string {
	ms := int64(math.Round(seconds * 1000))
	return fmt.Sprintf("%02d:%02d:%02d,%03d", ms/3600000, ms/60000%60, ms/1000%60, ms%1000)
}

type stagePreference struct {
	Layout          string  `json:"layout"`
	Background      string  `json:"background"`
	Scale           float64 `json:"scale"`
	Translation     float64 `json:"translation"`
	Dim             float64 `json:"dim"`
	Art             bool    `json:"art"`
	Glow            bool    `json:"glow"`
	Density         string  `json:"density"`
	Font            string  `json:"font"`
	TranslationFont string  `json:"translationFont"`
	Motion          string  `json:"motion"`
	Reveal          string  `json:"reveal"`
}

func (p stagePreference) valid() bool {
	return strings.Contains(" auto theater focus dual karaoke ", " "+p.Layout+" ") &&
		strings.Contains(" oled cover aura theme ", " "+p.Background+" ") &&
		strings.Contains(" surround single ", " "+p.Density+" ") &&
		(p.Reveal == "" || p.Reveal == "word" || p.Reveal == "character" || p.Reveal == "line") &&
		len(p.Font) <= 320 && len(p.TranslationFont) <= 320 && len(p.Motion) <= 100 &&
		!strings.ContainsAny(p.Font+p.TranslationFont, "\x00\r\n") &&
		p.Scale >= .65 && p.Scale <= 1.5 && p.Translation >= .35 && p.Translation <= 1 && p.Dim >= .35 && p.Dim <= .95
}

func validateV3Preference(key string, raw json.RawMessage) error {
	invalid := errors.New("قيمة غير صالحة في إعدادات نَوى 3")
	switch key {
	case "v3Stage":
		var designs struct {
			Global *stagePreference           `json:"global"`
			Tracks map[string]stagePreference `json:"tracks"`
			Albums map[string]stagePreference `json:"albums"`
		}
		if json.Unmarshal(raw, &designs) != nil || len(designs.Tracks)+len(designs.Albums) > 10000 ||
			designs.Global != nil && !designs.Global.valid() {
			return invalid
		}
		for _, entries := range []map[string]stagePreference{designs.Tracks, designs.Albums} {
			for id, design := range entries {
				if len(id) > 1200 || !design.valid() {
					return invalid
				}
			}
		}
	case "v3Cues":
		var entries map[string][]struct {
			Name string  `json:"name"`
			At   float64 `json:"at"`
		}
		if json.Unmarshal(raw, &entries) != nil || len(entries) > 10000 {
			return invalid
		}
		for id, items := range entries {
			if len(id) > 200 || len(items) > 24 {
				return invalid
			}
			for _, item := range items {
				if item.Name == "" || len(item.Name) > 320 || !finiteTime(item.At) || item.At > 86400 {
					return invalid
				}
			}
		}
	case "v3OfflineQuota":
		var bytes int64
		if json.Unmarshal(raw, &bytes) != nil || bytes <= 0 || bytes > 1<<40 {
			return invalid
		}
	}
	return nil
}
