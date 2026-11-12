package main

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type LyricEditDocument struct {
	TaskID   string     `json:"taskId"`
	Revision string     `json:"revision"`
	Source   string     `json:"source"`
	Text     string     `json:"text"`
	Timed    bool       `json:"timed"`
	Lines    []WordLine `json:"lines"`
	SavedAt  string     `json:"savedAt"`
}

func lyricEditRevision(c LyricCache) string { b, _ := json.Marshal(c); return key(string(b)) }
func lyricRecordText(c LyricCache) string {
	if c.Record.Synced != "" {
		return c.Record.Synced
	}
	return c.Record.Plain
}
func (s *Studio) GetLyricDocument(id string) (LyricEditDocument, error) {
	t, ok := s.task(id)
	if !ok {
		return LyricEditDocument{}, errors.New("اختر أغنية محفوظة")
	}
	s.translationMu.Lock()
	defer s.translationMu.Unlock()
	c, e := s.readLyricCache(t.Song)
	if e != nil {
		c.Song = t.Song
		c.Record.Provider = "local"
		if t.Lyrics != "" {
			if b, e := os.ReadFile(t.Lyrics); e == nil && len(b) <= 2<<20 {
				if validLRC(string(b), t.Song.Duration) {
					c.Record.Synced = string(b)
				} else {
					c.Record.Plain = string(b)
				}
			}
		}
	}
	return LyricEditDocument{TaskID: id, Revision: lyricEditRevision(c), Source: c.Record.Provider, Text: lyricRecordText(c), Timed: c.Record.Synced != "", Lines: c.Words, SavedAt: c.SavedAt}, nil
}
func linesLRC(lines []WordLine) string {
	var b strings.Builder
	for _, l := range lines {
		ms := int64(math.Round(l.Start * 1000))
		fmt.Fprintf(&b, "[%02d:%02d.%03d]%s\n", ms/60000, ms/1000%60, ms%1000, l.Text)
	}
	return b.String()
}
func validateEditedLines(lines []WordLine, duration float64) error {
	if len(lines) > 10000 {
		return errors.New("عدد سطور أكبر من الحد")
	}
	previous := -1.0
	for _, l := range lines {
		if strings.TrimSpace(l.Text) == "" || !finiteTime(l.Start) || !finiteTime(l.End) || l.End <= l.Start || l.Start < previous || duration > 0 && l.End > duration+.1 {
			return errors.New("راجع بداية ونهاية السطور وترتيبها")
		}
		previous = l.Start
		if len(l.Words) > 0 {
			if e := validateAlignment(alignmentOutput{Lines: []WordLine{l}, LRC: linesLRC([]WordLine{l})}, duration); e != nil {
				return e
			}
			end := -1.0
			for _, w := range l.Words {
				if w.Start < end-.025 {
					return errors.New("تداخل توقيت الكلمات؛ عدّل حدودها أولًا")
				}
				end = w.End
			}
		}
	}
	return nil
}
func finiteTime(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 }
func (s *Studio) SaveLyricDocument(doc LyricEditDocument) (LyricEditDocument, error) {
	t, ok := s.task(doc.TaskID)
	if !ok || t.Path == "" {
		return doc, errors.New("اختر أغنية محفوظة")
	}
	if len(doc.Text) > 2<<20 || len(doc.Lines) > 10000 {
		return doc, errors.New("ملف الكلمات أكبر من الحد")
	}
	s.translationMu.Lock()
	defer s.translationMu.Unlock()
	c, e := s.readLyricCache(t.Song)
	if e != nil {
		c = LyricCache{Song: t.Song, Record: LyricsRecord{Provider: "local"}}
		if t.Lyrics != "" {
			if b, e := os.ReadFile(t.Lyrics); e == nil {
				if validLRC(string(b), t.Song.Duration) {
					c.Record.Synced = string(b)
				} else {
					c.Record.Plain = string(b)
				}
			}
		}
	}
	if lyricEditRevision(c) != doc.Revision {
		return doc, errors.New("تغيّرت الكلمات أثناء التحرير؛ افتح أحدث نسخة قبل الحفظ")
	}
	text := strings.TrimSpace(doc.Text)
	record := LyricsRecord{Title: t.Song.Title, Artist: t.Song.Artist, Album: t.Song.Album, Duration: t.Song.Duration, Provider: "local"}
	if len(doc.Lines) > 0 {
		if e = validateEditedLines(doc.Lines, t.Song.Duration); e != nil {
			return doc, e
		}
		record.Synced = linesLRC(doc.Lines)
		record.Plain = stripLyricText(record.Synced)
		if text != "" && alignmentIdentity(stripLyricText(text)) != alignmentIdentity(record.Plain) {
			return doc, errors.New("النص لا يطابق سطور المحرّر")
		}
	} else {
		if text == "" {
			return doc, errors.New("أدخل الكلمات أولًا")
		}
		if doc.Timed {
			if !validLRC(text, t.Song.Duration) {
				return doc, errors.New("توقيت LRC غير صالح")
			}
			record.Synced = text
			record.Plain = stripLyricText(text)
		} else {
			record.Plain = text
		}
	}
	if e = s.backupLyricEdit(t, c); e != nil {
		return doc, e
	}
	next := LyricCache{Song: t.Song, Record: record}
	// Partial real word times remain partial, never filled by interpolation.
	anyWords := false
	for _, l := range doc.Lines {
		if len(l.Words) > 0 {
			anyWords = true
		}
	}
	if alignmentIdentity(stripLyricText(lyricRecordText(c))) == alignmentIdentity(record.Plain) {
		next.Pronunciation = c.Pronunciation
	}
	if anyWords {
		next.Words = doc.Lines
		next.Alignment = &AlignmentReport{Accepted: true, NeedsReview: true, TextMatched: true, CharacterTiming: "visual-within-word", Model: "manual-editor", Note: "توقيت عدّله المستخدم؛ ليس قياسًا صوتيًا للحروف"}
	}
	if e = s.writeLyricCache(next); e != nil {
		return doc, e
	}
	if _, e = s.persistLyricView(t, record); e != nil {
		return doc, e
	}
	next, _ = s.readLyricCache(t.Song)
	return LyricEditDocument{TaskID: t.ID, Revision: lyricEditRevision(next), Source: "local", Text: lyricRecordText(next), Timed: record.Synced != "", Lines: next.Words, SavedAt: next.SavedAt}, nil
}
func (s *Studio) backupLyricEdit(t Task, c LyricCache) error {
	if c.Record.Synced == "" && c.Record.Plain == "" {
		return nil
	}
	dir := filepath.Join(s.root, "data", "lyrics-history", lyricKey(t.Song))
	b, e := json.Marshal(c)
	if e != nil {
		return e
	}
	if e = writeChanged(filepath.Join(dir, fmt.Sprintf("%019d.json", time.Now().UnixNano())), b); e != nil {
		return e
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	sort.Strings(files)
	for len(files) > 30 {
		if e = os.Remove(files[0]); e != nil {
			return e
		}
		files = files[1:]
	}
	return nil
}

type LyricHistoryItem struct {
	ID        string `json:"id"`
	SavedAt   string `json:"savedAt"`
	Source    string `json:"source"`
	WordTimed bool   `json:"wordTimed"`
}

func (s *Studio) GetLyricHistory(id string) ([]LyricHistoryItem, error) {
	t, ok := s.task(id)
	if !ok {
		return nil, errors.New("المسار غير موجود")
	}
	files, _ := filepath.Glob(filepath.Join(s.root, "data", "lyrics-history", lyricKey(t.Song), "*.json"))
	sort.Sort(sort.Reverse(sort.StringSlice(files)))
	out := []LyricHistoryItem{}
	for _, p := range files {
		b, e := os.ReadFile(p)
		if e != nil {
			continue
		}
		var c LyricCache
		if json.Unmarshal(b, &c) == nil {
			out = append(out, LyricHistoryItem{ID: strings.TrimSuffix(filepath.Base(p), ".json"), SavedAt: c.SavedAt, Source: c.Record.Provider, WordTimed: len(c.Words) > 0})
		}
	}
	return out, nil
}
func (s *Studio) RestoreLyricRevision(id, revision, expected string) (LyricEditDocument, error) {
	t, ok := s.task(id)
	if !ok {
		return LyricEditDocument{}, errors.New("المسار غير موجود")
	}
	if len(revision) != 19 || strings.Trim(revision, "0123456789") != "" {
		return LyricEditDocument{}, errors.New("نسخة كلمات غير صالحة")
	}
	p := filepath.Join(s.root, "data", "lyrics-history", lyricKey(t.Song), revision+".json")
	raw, e := os.ReadFile(p)
	if e != nil {
		return LyricEditDocument{}, e
	}
	var c LyricCache
	if e = json.Unmarshal(raw, &c); e != nil {
		return LyricEditDocument{}, e
	}
	return s.SaveLyricDocument(LyricEditDocument{TaskID: id, Revision: expected, Text: lyricRecordText(c), Timed: c.Record.Synced != "", Lines: c.Words})
}

type LyricSourcePreview struct {
	Provider string       `json:"provider"`
	Rank     int          `json:"rank"`
	Record   LyricsRecord `json:"record"`
	Words    []WordLine   `json:"words,omitempty"`
}

func (s *Studio) PreviewLyricSource(id, provider string) (LyricSourcePreview, error) {
	t, ok := s.task(id)
	if !ok {
		return LyricSourcePreview{}, errors.New("اختر مسارًا محفوظًا")
	}
	enabled := false
	for _, p := range s.GetState().Preferences.LyricProviders {
		if p == provider {
			enabled = true
		}
	}
	if !enabled {
		return LyricSourcePreview{}, errors.New("فعّل المصدر من الإعدادات أولًا")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	r, e := s.fetchLyricProvider(ctx, t.Song, provider)
	return LyricSourcePreview{Provider: provider, Rank: lyricRecordRank(r, t.Song), Record: r, Words: r.Words}, e
}

type Waveform struct {
	Duration float64   `json:"duration"`
	Peaks    []float64 `json:"peaks"`
}

func (s *Studio) GetWaveform(id string) (Waveform, error) {
	t, ok := s.task(id)
	if !ok || t.Status != "done" || t.Path == "" {
		return Waveform{}, errors.New("الموجة الصوتية تحتاج ملفًا محفوظًا")
	}
	if t.Song.Duration <= 0 || t.Song.Duration > 4*3600 {
		return Waveform{}, errors.New("المدة غير مناسبة لإنشاء الموجة")
	}
	st, e := os.Stat(t.Path)
	if e != nil {
		return Waveform{}, e
	}
	p := filepath.Join(s.root, "data", "waveforms", key(id+fmt.Sprint(st.Size(), st.ModTime().UnixNano()))+".json")
	if b, e := os.ReadFile(p); e == nil {
		var w Waveform
		if json.Unmarshal(b, &w) == nil {
			return w, nil
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	release, e := s.acquireHeavyTask(ctx, "waveform")
	if e != nil {
		return Waveform{}, e
	}
	defer release()
	cmd := s.command(ctx, "ffmpeg", "-v", "error", "-nostdin", "-i", t.Path, "-map", "0:a:0", "-ac", "1", "-ar", "200", "-f", "u8", "pipe:1")
	out := limitedBuffer{Limit: 3 << 20}
	stderr := limitedBuffer{Limit: 1000}
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if e = cmd.Run(); e != nil {
		return Waveform{}, errors.New("تعذر تجهيز الموجة الصوتية؛ يمكنك تعديل التوقيت بالأرقام")
	}
	data := out.Bytes()
	if len(data) == 0 {
		return Waveform{}, errors.New("لا توجد بيانات صوتية")
	}
	bins := min(2400, len(data))
	w := Waveform{Duration: t.Song.Duration, Peaks: make([]float64, bins)}
	for i, v := range data {
		k := i * bins / len(data)
		a := math.Abs(float64(v)-128) / 128
		if a > w.Peaks[k] {
			w.Peaks[k] = a
		}
	}
	b, _ := json.Marshal(w)
	if e = writeChanged(p, b); e != nil {
		return Waveform{}, e
	}
	return w, nil
}
func (s *Studio) ExportLyricsDocument(id, format string) (string, error) {
	d, e := s.GetLyricDocument(id)
	if e != nil {
		return "", e
	}
	switch format {
	case "json":
		b, e := json.MarshalIndent(d, "", "  ")
		return string(b), e
	case "txt":
		return stripLyricText(d.Text), nil
	case "srt":
		return s.exportSRT(id, d)
	case "lrc":
		if !d.Timed {
			return "", errors.New("هذه الكلمات غير موقّتة")
		}
		return d.Text, nil
	case "ttml":
		if len(d.Lines) == 0 {
			return "", errors.New("تصدير TTML يتطلب توقيت كلمات")
		}
		var b strings.Builder
		b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><tt xmlns="http://www.w3.org/ns/ttml"><body><div>`)
		for _, l := range d.Lines {
			fmt.Fprintf(&b, `<p begin="%.3fs" end="%.3fs">`, l.Start, l.End)
			for i, w := range l.Words {
				if i > 0 {
					b.WriteByte(' ')
				}
				fmt.Fprintf(&b, `<span begin="%.3fs" end="%.3fs">`, w.Start, w.End)
				xml.EscapeText(&b, []byte(w.Text))
				b.WriteString("</span>")
			}
			if len(l.Words) == 0 {
				xml.EscapeText(&b, []byte(l.Text))
			}
			b.WriteString("</p>")
		}
		b.WriteString("</div></body></tt>")
		return b.String(), nil
	}
	return "", errors.New("صيغة تصدير غير مدعومة")
}
