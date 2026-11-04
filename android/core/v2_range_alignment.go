package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type RangeAlignmentPreview struct {
	TaskID   string          `json:"taskId"`
	Revision string          `json:"revision"`
	Start    float64         `json:"start"`
	End      float64         `json:"end"`
	Lines    []WordLine      `json:"lines"`
	Report   AlignmentReport `json:"report"`
}

func (s *Studio) StartRangeAlignment(id string, start, end float64, language string) error {
	t, ok := s.task(id)
	if !ok || t.Status != "done" || !finiteTime(start) || !finiteTime(end) || end <= start || end-start > 120 || end > t.Song.Duration+.1 {
		return errors.New("اختر نطاقًا صالحًا داخل الأغنية لا يزيد عن دقيقتين")
	}
	if !s.alignmentReady() {
		return errors.New("جهّز نموذج التوقيت أولًا")
	}
	doc, e := s.GetLyricDocument(id)
	if e != nil {
		return e
	}
	source, e := s.translationOriginal(t)
	if e != nil {
		return e
	}
	selected := []string{}
	for _, r := range source {
		if r.Time != nil && *r.Time >= start && *r.Time < end {
			selected = append(selected, r.Original)
		}
	}
	text := strings.Join(selected, "\n")
	if len(strings.TrimSpace(text)) < 3 {
		return errors.New("هذا النطاق لا يحتوي سطورًا بتوقيت محفوظ؛ حدّد توقيت السطور أولًا")
	}
	s.mu.Lock()
	if s.alignState.Running {
		s.mu.Unlock()
		return errors.New("يوجد تحليل قيد التنفيذ")
	}
	s.alignState = AlignmentState{Range: true, Running: true, Ready: true, TaskID: id, Phase: "تحليل النطاق المحدد"}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	s.alignCancel = cancel
	s.mu.Unlock()
	go func() {
		defer cancel()
		e := s.runRangeAlignment(ctx, t, doc.Revision, start, end, text, language)
		s.mu.Lock()
		s.alignState.Running = false
		s.alignCancel = nil
		if e != nil {
			s.alignState.Error = e.Error()
			s.alignState.Phase = "لم يتغير التوقيت المحفوظ"
		}
		s.mu.Unlock()
	}()
	return nil
}
func (s *Studio) runRangeAlignment(ctx context.Context, t Task, revision string, start, end float64, text, language string) error {
	release, e := s.acquireHeavyTask(ctx, "alignment-range")
	if e != nil {
		return e
	}
	defer release()
	dir := filepath.Join(s.root, "data", "range-alignment")
	if e = os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	audio := filepath.Join(dir, t.ID+".wav")
	defer os.Remove(audio)
	cmd := s.command(ctx, "ffmpeg", "-v", "error", "-nostdin", "-y", "-ss", formatSeconds(start), "-i", t.Path, "-t", formatSeconds(end-start), "-map", "0:a:0", "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", audio)
	if e = cmd.Run(); e != nil {
		return errors.New("تعذر تجهيز صوت النطاق المحدد")
	}
	result, e := s.computeRangeAlignment(ctx, audio, text, language, end-start)
	if e != nil {
		return e
	}
	if e = validateAlignment(result, end-start); e != nil {
		return e
	}
	if alignmentIdentity(text) != alignmentIdentity(stripLyricText(result.LRC)) {
		return errors.New("الكلمات المحللة لا تطابق النطاق؛ الأصل محفوظ")
	}
	for i := range result.Lines {
		l := &result.Lines[i]
		l.Start += start
		l.End += start
		for j := range l.Words {
			l.Words[j].Start += start
			l.Words[j].End += start
		}
	}
	result.Report.NeedsReview = true
	result.Report.Accepted = false
	preview := RangeAlignmentPreview{TaskID: t.ID, Revision: revision, Start: start, End: end, Lines: result.Lines, Report: result.Report}
	b, _ := json.Marshal(preview)
	if e = writeChanged(filepath.Join(dir, t.ID+".json"), b); e != nil {
		return e
	}
	s.mu.Lock()
	s.alignState.Progress = 1
	s.alignState.Phase = "نطاق محلّل يحتاج مراجعتك"
	s.alignState.Report = &result.Report
	s.mu.Unlock()
	return nil
}
func (s *Studio) GetRangeAlignmentPreview(id string) (RangeAlignmentPreview, error) {
	t, ok := s.task(id)
	if !ok {
		return RangeAlignmentPreview{}, errors.New("المسار غير موجود")
	}
	b, e := os.ReadFile(filepath.Join(s.root, "data", "range-alignment", t.ID+".json"))
	if e != nil {
		return RangeAlignmentPreview{}, e
	}
	var p RangeAlignmentPreview
	e = json.Unmarshal(b, &p)
	if e != nil {
		return p, e
	}
	doc, e := s.GetLyricDocument(id)
	if e != nil {
		return p, e
	}
	if p.TaskID != id || p.Revision != doc.Revision {
		return p, errors.New("تغيّرت الكلمات منذ تحليل النطاق؛ أعد التحليل")
	}
	return p, validateEditedLines(p.Lines, t.Song.Duration)
}
func (s *Studio) ApplyRangeAlignment(id string) (LyricEditDocument, error) {
	p, e := s.GetRangeAlignmentPreview(id)
	if e != nil {
		return LyricEditDocument{}, e
	}
	doc, e := s.GetLyricDocument(id)
	if e != nil {
		return doc, e
	}
	t, _ := s.task(id)
	source, e := s.translationOriginal(t)
	if e != nil {
		return doc, e
	}
	base := doc.Lines
	if len(base) == 0 {
		for i, r := range source {
			if r.Time == nil {
				return doc, errors.New("الأصل يحتوي سطورًا بلا توقيت")
			}
			stop := t.Song.Duration
			if i+1 < len(source) && source[i+1].Time != nil {
				stop = *source[i+1].Time
			}
			base = append(base, WordLine{Start: *r.Time, End: stop, Text: r.Original})
		}
	}
	merged := []WordLine{}
	inserted := false
	for _, l := range base {
		if l.Start >= p.Start && l.Start < p.End {
			if !inserted {
				merged = append(merged, p.Lines...)
				inserted = true
			}
			continue
		}
		merged = append(merged, l)
	}
	if !inserted {
		return doc, errors.New("لم تعد سطور النطاق موجودة")
	}
	doc.Lines = merged
	doc.Timed = true
	next, e := s.SaveLyricDocument(doc)
	if e == nil {
		s.mu.Lock()
		if s.alignState.TaskID == id {
			s.alignState.Phase = "اعتمدت توقيت النطاق"
			if s.alignState.Report != nil {
				s.alignState.Report.Accepted = true
			}
		}
		s.mu.Unlock()
	}
	return next, e
}

func formatSeconds(v float64) string { return strconv.FormatFloat(v, 'f', 3, 64) }
