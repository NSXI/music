package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	unicodeNorm "golang.org/x/text/unicode/norm"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type AlignmentReport struct {
	Accepted           bool    `json:"accepted,omitempty"`
	Coverage           float64 `json:"coverage"`
	MeanProbability    float64 `json:"meanProbability"`
	Words              int     `json:"words"`
	ValidWords         int     `json:"validWords"`
	NeedsReview        bool    `json:"needsReview"`
	Language           string  `json:"language"`
	Model              string  `json:"model"`
	Note               string  `json:"note"`
	Refined            bool    `json:"refined"`
	ResolutionSeconds  float64 `json:"resolutionSeconds"`
	TextMatched        bool    `json:"textMatched"`
	OverlapWords       int     `json:"overlapWords"`
	InstantWords       int     `json:"instantWords"`
	LowConfidenceWords int     `json:"lowConfidenceWords"`
	CharacterTiming    string  `json:"characterTiming"`
	Warning            string  `json:"warning,omitempty"`
}
type AlignmentState struct {
	Range     bool             `json:"range,omitempty"`
	Pending   int              `json:"pending"`
	Running   bool             `json:"running"`
	Ready     bool             `json:"ready"`
	TaskID    string           `json:"taskId"`
	Progress  float64          `json:"progress"`
	Phase     string           `json:"phase"`
	Error     string           `json:"error"`
	Report    *AlignmentReport `json:"report,omitempty"`
	Candidate string           `json:"candidate,omitempty"`
}
type alignmentOutput struct {
	LRC            string           `json:"lrc"`
	Lines          []WordLine       `json:"lines"`
	Report         AlignmentReport  `json:"report"`
	Original       string           `json:"original"`
	OriginalTimed  bool             `json:"originalTimed"`
	OriginalSource string           `json:"originalSource"`
	OriginalWords  []WordLine       `json:"originalWords,omitempty"`
	OriginalReport *AlignmentReport `json:"originalReport,omitempty"`
}

func alignmentIdentity(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, unicodeNorm.NFC.String(text))
}

// Validate model data at both boundaries; confidence cannot make invalid times safe.
func validateAlignment(result alignmentOutput, duration float64) error {
	if len(result.Lines) == 0 {
		return errors.New("لم يُرجع المحسن كلمات بتوقيت صالح")
	}
	previousStart := -1.0
	count := 0
	text := ""
	for _, line := range result.Lines {
		if math.IsNaN(line.Start) || math.IsInf(line.Start, 0) || math.IsNaN(line.End) || math.IsInf(line.End, 0) || line.Start < 0 || line.End <= line.Start || len(line.Words) == 0 {
			return errors.New("حدود سطر غير صالحة؛ أعد تحليل التوقيت")
		}
		joined := ""
		lineStart := -1.0
		for _, word := range line.Words {
			if math.IsNaN(word.Start) || math.IsInf(word.Start, 0) || math.IsNaN(word.End) || math.IsInf(word.End, 0) || math.IsNaN(word.Probability) || math.IsInf(word.Probability, 0) || word.Probability < 0 || word.Probability > 1 || word.Start < 0 || word.End <= word.Start || word.Start < previousStart || word.Start < line.Start-.025 || word.End > line.End+.025 || (duration > 0 && word.End > duration+.1) {
				return errors.New("توقيت كلمة غير صالح؛ احتُفظ بالأصل")
			}
			if lineStart < 0 {
				lineStart = word.Start
			}
			previousStart = word.Start
			joined += word.Text
			count++
		}
		if alignmentIdentity(joined) != alignmentIdentity(line.Text) {
			return errors.New("كلمات التحليل لا تطابق السطر الأصلي")
		}
		text += line.Text
	}
	if alignmentIdentity(stripLyricText(result.LRC)) != alignmentIdentity(text) {
		return errors.New("نص السطور لا يطابق توقيت الكلمات")
	}
	if result.Original != "" && alignmentIdentity(text) != alignmentIdentity(stripLyricText(result.Original)) {
		return errors.New("نص التحليل لا يطابق الكلمات الأصلية؛ احتُفظ بالأصل")
	}
	if count == 0 {
		return errors.New("التحليل خالٍ من الكلمات")
	}
	return nil
}

func (s *Studio) alignmentWorker() string {
	path := filepath.Join(s.root, "tools", "aligner", "MadarAlignV2.exe")
	if _, e := os.Stat(path); e == nil {
		return path
	}
	return filepath.Join(s.root, "tools", "aligner", "MadarAlign.exe")
}

func (s *Studio) alignmentReady() bool {
	_, e := os.Stat(s.alignmentWorker())
	_, m := os.Stat(filepath.Join(s.root, "tools", "models", "base.pt"))
	return e == nil && m == nil
}
func (s *Studio) StartAlignment(id, language string) error {
	t, ok := s.task(id)
	if !ok || t.Path == "" {
		return errors.New("نزّل الأغنية أولًا أو أضف ملفًا محليًا")
	}
	if !s.alignmentReady() {
		return errors.New(s.missingAlignmentMessage())
	}
	s.mu.Lock()
	if s.alignState.Running {
		s.mu.Unlock()
		return errors.New("توجد أغنية قيد المزامنة الآن")
	}
	s.alignState = AlignmentState{Running: true, Ready: true, TaskID: id, Phase: "تجهيز النص والصوت"}
	ctx := s.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	run, cancel := context.WithTimeout(ctx, 30*time.Minute)
	s.alignCancel = cancel
	s.mu.Unlock()
	go func() {
		defer cancel()
		e := s.runAlignment(run, t, language)
		s.mu.Lock()
		s.alignState.Running = false
		if e != nil {
			s.alignState.Error = e.Error()
			s.alignState.Phase = "تعذرت المزامنة"
		}
		s.alignCancel = nil
		s.mu.Unlock()
		s.nextAlignment()
	}()
	return nil
}
func (s *Studio) CancelAlignment() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.alignCancel != nil {
		s.alignCancel()
	}
}
func stripLyricText(text string) string {
	stamp := regexp.MustCompile(`\[[^\]]*\]|<\d{1,3}:\d{2}(?:\.\d+)?>`)
	tags := regexp.MustCompile(`\[(\d{1,3}):(\d{2}(?:\.\d+)?)\]`)
	header := regexp.MustCompile(`(?i)^\[(ar|ti|al|by|offset|length|re|ve):`)
	type occurrence struct {
		at   float64
		text string
	}
	var timed []occurrence
	allTimed := true
	rows := []string{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if header.MatchString(line) {
			continue
		}
		matches := tags.FindAllStringSubmatch(line, -1)
		line = strings.TrimSpace(stamp.ReplaceAllString(line, ""))
		if line != "" {
			rows = append(rows, line)
			valid := 0
			for _, match := range matches {
				minutes, _ := strconv.ParseFloat(match[1], 64)
				seconds, _ := strconv.ParseFloat(match[2], 64)
				if seconds >= 60 {
					continue
				}
				timed = append(timed, occurrence{minutes*60 + seconds, line})
				valid++
			}
			if valid == 0 {
				allTimed = false
			}
		}
	}
	// Repeated LRC tags represent repeated sung text. Feed the aligner the full
	// chronological transcript rather than dropping later chorus occurrences.
	if allTimed && len(timed) > 0 {
		sort.SliceStable(timed, func(i, j int) bool { return timed[i].at < timed[j].at })
		rows = rows[:0]
		for _, item := range timed {
			rows = append(rows, item.text)
		}
	}
	return strings.Join(rows, "\n")
}
func (s *Studio) runAlignment(ctx context.Context, t Task, language string) error {
	release, slotErr := s.acquireHeavyTask(ctx, "alignment")
	if slotErr != nil {
		return slotErr
	}
	defer release()
	var original LyricView
	var originalWords []WordLine
	var originalReport *AlignmentReport
	if cache, e := s.readLyricCache(t.Song); e == nil {
		r := cache.Record
		originalWords = cache.Words
		originalReport = cache.Alignment
		text := r.Plain
		timed := false
		if r.Synced != "" {
			text = r.Synced
			timed = true
		}
		original = LyricView{Text: text, Timed: timed, Source: r.Provider}
	} else if t.Lyrics != "" {
		b, e := os.ReadFile(t.Lyrics)
		if e != nil {
			return e
		}
		original = LyricView{Text: string(b), Timed: validLRC(string(b), t.Song.Duration), Source: t.LyricsSource}
	}
	text := stripLyricText(original.Text)
	if len([]rune(text)) < 8 {
		return errors.New("لا يوجد نص كافٍ للمزامنة. اجلب الكلمات أو أضف ملف TXT/LRC أولًا")
	}
	dir := filepath.Join(s.root, "data", "alignment")
	os.MkdirAll(dir, 0755)
	input := filepath.Join(dir, t.ID+"-input.json")
	output := filepath.Join(dir, t.ID+"-result.json")
	request := map[string]any{"audio": t.Path, "text": text, "language": language, "model": filepath.Join(s.root, "tools", "models", "base.pt"), "output": output, "ffmpeg": filepath.Join(s.root, "tools", "ffmpeg.exe")}
	raw, _ := json.Marshal(request)
	if e := os.WriteFile(input, raw, 0600); e != nil {
		return e
	}
	cmd := hiddenCommand(ctx, s.alignmentWorker(), input)
	cmd.Env = append(os.Environ(), "HF_HUB_OFFLINE=1", "TRANSFORMERS_OFFLINE=1")
	stdout, e := cmd.StdoutPipe()
	if e != nil {
		return e
	}
	var stderr limitedBuffer
	stderr.Limit = 12000
	cmd.Stderr = &stderr
	cmd.Env = withWorkerEnvironment(cmd.Env, s.backgroundWorkerEnv())
	if e = cmd.Start(); e != nil {
		return e
	}
	s.applyWorkerBudget(cmd)
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		var update struct {
			Progress float64 `json:"progress"`
			Phase    string  `json:"phase"`
		}
		if json.Unmarshal(scanner.Bytes(), &update) == nil {
			s.mu.Lock()
			s.alignState.Progress = update.Progress
			s.alignState.Phase = update.Phase
			s.mu.Unlock()
		}
	}
	e = cmd.Wait()
	if ctx.Err() != nil {
		return errors.New("توقفت المزامنة؛ احتُفظ بالكلمات الأصلية")
	}
	if e != nil {
		return fmt.Errorf("محرك المزامنة: %s", trimError(stderr.Bytes()))
	}
	raw, e = os.ReadFile(output)
	if e != nil {
		return e
	}
	var result alignmentOutput
	if e = json.Unmarshal(raw, &result); e != nil {
		return e
	}
	if !validLRC(result.LRC, t.Song.Duration) {
		return errors.New("النموذج لم يُرجع توقيتًا صالحًا. احتُفظ بالأصل")
	}
	result.Original = original.Text
	result.OriginalTimed = original.Timed
	result.OriginalSource = original.Source
	result.OriginalWords = originalWords
	result.OriginalReport = originalReport
	if e = validateAlignment(result, t.Song.Duration); e != nil {
		return e
	}
	if !result.Report.Refined {
		result.Report.NeedsReview = true
	}
	raw, _ = json.MarshalIndent(result, "", "  ")
	if e = os.WriteFile(output, raw, 0600); e != nil {
		return e
	}
	candidate := filepath.Join(dir, t.ID+".aligned.lrc")
	os.WriteFile(candidate, []byte(result.LRC), 0600)
	s.mu.Lock()
	s.alignState.Progress = 1
	s.alignState.Report = &result.Report
	s.alignState.Candidate = result.LRC
	s.alignState.Phase = "اكتملت المزامنة"
	s.mu.Unlock()
	if result.Report.NeedsReview {
		s.mu.Lock()
		s.alignState.Phase = "نتيجة تحتاج مراجعة؛ الأصل محفوظ"
		s.mu.Unlock()
		return nil
	}
	return s.ApplyAlignment(t.ID)
}

type AlignmentPreview struct {
	Lines  []WordLine      `json:"lines"`
	Report AlignmentReport `json:"report"`
}

func (s *Studio) GetAlignmentPreview(id string) (AlignmentPreview, error) {
	t, ok := s.task(id)
	if !ok {
		return AlignmentPreview{}, errors.New("المسار غير موجود")
	}
	raw, e := os.ReadFile(filepath.Join(s.root, "data", "alignment", id+"-result.json"))
	if e != nil {
		return AlignmentPreview{}, e
	}
	var result alignmentOutput
	if e = json.Unmarshal(raw, &result); e != nil {
		return AlignmentPreview{}, e
	}
	if e = validateAlignment(result, t.Song.Duration); e != nil {
		return AlignmentPreview{}, e
	}
	return AlignmentPreview{Lines: result.Lines, Report: result.Report}, nil
}
func (s *Studio) ApplyAlignment(id string) error {
	t, ok := s.task(id)
	if !ok {
		return errors.New("المسار غير موجود")
	}
	raw, e := os.ReadFile(filepath.Join(s.root, "data", "alignment", id+"-result.json"))
	if e != nil {
		return e
	}
	var result alignmentOutput
	if e = json.Unmarshal(raw, &result); e != nil {
		return e
	}
	if !validLRC(result.LRC, t.Song.Duration) {
		return errors.New("التوقيت غير صالح")
	}
	if e = validateAlignment(result, t.Song.Duration); e != nil {
		return e
	}
	if cached, err := s.readLyricCache(t.Song); err == nil {
		current := cached.Record.Plain
		if cached.Record.Synced != "" {
			current = cached.Record.Synced
		}
		if result.Original != "" && alignmentIdentity(stripLyricText(current)) != alignmentIdentity(stripLyricText(result.Original)) {
			return errors.New("تغيّر نص الكلمات بعد التحليل؛ أعد المزامنة قبل الاعتماد")
		}
	}
	if t.Lyrics != "" {
		if b, e := os.ReadFile(t.Lyrics); e == nil {
			backup := t.Lyrics + ".source-backup"
			if _, e = os.Stat(backup); os.IsNotExist(e) {
				os.WriteFile(backup, b, 0600)
			}
		}
	}
	record := LyricsRecord{Title: t.Song.Title, Artist: t.Song.Artist, Album: t.Song.Album, Duration: t.Song.Duration, Synced: result.LRC, Plain: stripLyricText(result.LRC), Provider: "local-align"}
	result.Report.Accepted = true
	if e = s.writeLyricCache(LyricCache{Song: t.Song, Record: record, Words: result.Lines, Alignment: &result.Report}); e != nil {
		return e
	}
	_, e = s.persistLyricView(t, record)
	if e == nil {
		s.mu.Lock()
		if s.alignState.TaskID == id {
			report := result.Report
			s.alignState.Report = &report
			s.alignState.Phase = "توقيت معتمد باختيارك"
		}
		s.mu.Unlock()
	}
	return e
}
func (s *Studio) RestoreLyrics(id string) error {
	t, ok := s.task(id)
	if !ok {
		return errors.New("المسار غير موجود")
	}
	raw, e := os.ReadFile(filepath.Join(s.root, "data", "alignment", id+"-result.json"))
	if e != nil {
		return e
	}
	var result alignmentOutput
	if e = json.Unmarshal(raw, &result); e != nil {
		return e
	}
	record := LyricsRecord{Title: t.Song.Title, Artist: t.Song.Artist, Album: t.Song.Album, Duration: t.Song.Duration, Provider: result.OriginalSource}
	if result.OriginalTimed {
		record.Synced = result.Original
	} else {
		record.Plain = result.Original
	}
	if e = s.writeLyricCache(LyricCache{Song: t.Song, Record: record, Words: result.OriginalWords, Alignment: result.OriginalReport}); e != nil {
		return e
	}
	_, e = s.persistLyricView(t, record)
	return e
}
