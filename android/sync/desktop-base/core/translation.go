package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var translationLanguages = map[string]bool{"ar": true, "en": true, "fr": true, "es": true, "de": true, "ru": true, "ja": true, "ko": true, "zh": true, "hi": true, "tr": true, "pt": true, "it": true, "fa": true, "ur": true, "id": true, "nl": true, "uk": true}

type TranslationLine struct {
	Original string   `json:"original"`
	Text     string   `json:"text"`
	Time     *float64 `json:"time"`
}
type TranslationView struct {
	Lines           []TranslationLine `json:"lines"`
	Source          string            `json:"source"`
	Target          string            `json:"target"`
	RequestedSource string            `json:"requestedSource"`
	OriginalHash    string            `json:"originalHash"`
	Engine          string            `json:"engine"`
	Reviewed        bool              `json:"reviewed"`
	SameLanguage    bool              `json:"sameLanguage"`
	SavedAt         string            `json:"savedAt"`
	Status          string            `json:"status"`
	Exists          bool              `json:"exists"`
}
type TranslationRequest struct {
	TaskID string `json:"taskId"`
	Target string `json:"target"`
	Source string `json:"source"`
	Hash   string `json:"hash"`
}
type TranslationState struct {
	Running  bool    `json:"running"`
	Ready    bool    `json:"ready"`
	Pending  int     `json:"pending"`
	TaskID   string  `json:"taskId"`
	Target   string  `json:"target"`
	Progress float64 `json:"progress"`
	Phase    string  `json:"phase"`
	Error    string  `json:"error"`
	Revision int64   `json:"revision"`
}

func (s *Studio) translationReady() bool {
	for _, p := range []string{"tools/translator/MadarTranslate.exe", "tools/models/nllb-int8/model.bin", "tools/models/nllb-int8/tokenizer.json"} {
		if _, e := os.Stat(filepath.Join(s.root, filepath.FromSlash(p))); e != nil {
			return false
		}
	}
	return true
}
func translationRows(text string, timed bool) []TranslationLine {
	rows := []TranslationLine{}
	stamp := regexp.MustCompile(`\[(\d{1,3}):(\d{2}(?:\.\d+)?)\]`)
	tags := regexp.MustCompile(`\[[^\]]*\]|<\d{1,3}:\d{2}(?:\.\d+)?>`)
	offset := 0.0
	if m := regexp.MustCompile(`(?i)\[offset:([+-]?\d+)\]`).FindStringSubmatch(text); len(m) > 1 {
		offset, _ = strconv.ParseFloat(m[1], 64)
		offset /= 1000
	}
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r", ""), "\n") {
		if !timed {
			if v := strings.TrimSpace(line); v != "" {
				rows = append(rows, TranslationLine{Original: v})
			}
			continue
		}
		body := strings.TrimSpace(tags.ReplaceAllString(line, ""))
		if body == "" {
			continue
		}
		for _, m := range stamp.FindAllStringSubmatch(line, -1) {
			minutes, _ := strconv.ParseFloat(m[1], 64)
			seconds, _ := strconv.ParseFloat(m[2], 64)
			if seconds >= 60 {
				continue
			}
			at := minutes*60 + seconds + offset
			rows = append(rows, TranslationLine{Original: body, Time: &at})
		}
	}
	if timed {
		sort.SliceStable(rows, func(i, j int) bool { return *rows[i].Time < *rows[j].Time })
	}
	return rows
}
func translationHash(rows []TranslationLine) string {
	texts := []string{}
	for _, r := range rows {
		texts = append(texts, r.Original)
	}
	b, _ := json.Marshal(texts)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func (s *Studio) translationOriginal(t Task) ([]TranslationLine, error) {
	text := ""
	timed := false
	if c, e := s.readLyricCache(t.Song); e == nil {
		text = c.Record.Plain
		if validLRC(c.Record.Synced, t.Song.Duration) {
			text = c.Record.Synced
			timed = true
		}
	}
	if text == "" && t.Lyrics != "" {
		if b, e := os.ReadFile(t.Lyrics); e == nil {
			text = string(b)
			timed = validLRC(text, t.Song.Duration)
		}
	}
	rows := translationRows(text, timed)
	if len(rows) == 0 {
		return nil, errors.New("أضف الكلمات أو اجلبها أولًا")
	}
	if len(rows) > 3000 || len(text) > 2<<20 {
		return nil, errors.New("نص الكلمات أطول من الحد المتاح")
	}
	return rows, nil
}
func validTranslationLanguage(target, source string) bool {
	return translationLanguages[target] && (source == "auto" || translationLanguages[source])
}
func (s *Studio) translationPath(t Task, hash, target, source string) string {
	return filepath.Join(s.root, "data", "lyrics", "translations", lyricKey(t.Song)+"-"+hash+"-"+target+"-"+source+".json")
}
func (s *Studio) readTranslation(t Task, rows []TranslationLine, target, source string) (TranslationView, error) {
	var v TranslationView
	b, e := os.ReadFile(s.translationPath(t, translationHash(rows), target, source))
	if e != nil {
		return v, e
	}
	if e = json.Unmarshal(b, &v); e != nil {
		return v, e
	}
	if v.OriginalHash != translationHash(rows) || v.Target != target || v.RequestedSource != source || len(v.Lines) != len(rows) {
		return v, errors.New("تغير نص الكلمات؛ يلزم ترجمة جديدة")
	}
	for i := range rows {
		if v.Lines[i].Original != rows[i].Original || strings.TrimSpace(v.Lines[i].Text) == "" {
			return v, errors.New("ترجمة محفوظة لا تطابق الكلمات")
		}
		v.Lines[i].Time = rows[i].Time
	}
	v.Exists = true
	v.Status = "ترجمة آلية محفوظة محليًا · قابلة للتصحيح"
	if v.Reviewed {
		v.Status = "تصحيح محفوظ محليًا"
	}
	if v.SameLanguage {
		v.Status = "الكلمات أصلًا باللغة المختارة"
	}
	return v, nil
}
func translationSidecar(v TranslationView) string {
	var out strings.Builder
	for _, r := range v.Lines {
		if r.Time != nil {
			at := math.Max(0, *r.Time)
			centis := int(math.Round(at * 100))
			fmt.Fprintf(&out, "[%02d:%02d.%02d]", centis/6000, centis/100%60, centis%100)
		}
		out.WriteString(strings.ReplaceAll(strings.ReplaceAll(r.Text, "\n", " "), "\r", " "))
		out.WriteByte('\n')
	}
	return out.String()
}
func (s *Studio) saveTranslation(t Task, v TranslationView) error {
	v.SavedAt = time.Now().Format(time.RFC3339Nano)
	v.Exists = true
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	path := s.translationPath(t, v.OriginalHash, v.Target, v.RequestedSource)
	if old, e := os.ReadFile(path); e == nil {
		var prev TranslationView
		if json.Unmarshal(old, &prev) == nil && prev.Reviewed && !v.Reviewed {
			if e = writeChanged(path+".previous", old); e != nil {
				return e
			}
		}
	}
	if e = writeChanged(path, b); e != nil {
		return e
	}
	return s.translationSidecars(t, v)
}
func (s *Studio) translationSidecars(t Task, v TranslationView) error {
	if t.Path == "" {
		return nil
	}
	base := strings.TrimSuffix(t.Path, filepath.Ext(t.Path)) + ".translation." + v.Target
	b, _ := json.MarshalIndent(v, "", "  ")
	if e := writeChanged(base+".json", b); e != nil {
		return fmt.Errorf("حُفظت الترجمة في الكاش؛ تعذر حفظ الملف الجانبي: %w", e)
	}
	ext := ".txt"
	if len(v.Lines) > 0 && v.Lines[0].Time != nil {
		ext = ".lrc"
	}
	return writeChanged(base+ext, []byte(translationSidecar(v)))
}
func (s *Studio) GetTranslation(id, target, source string, automatic bool) (TranslationView, error) {
	if !validTranslationLanguage(target, source) {
		return TranslationView{}, errors.New("لغة ترجمة غير مدعومة")
	}
	t, ok := s.task(id)
	if !ok {
		return TranslationView{}, errors.New("المسار غير موجود")
	}
	rows, e := s.translationOriginal(t)
	if e != nil {
		return TranslationView{}, e
	}
	s.translationMu.Lock()
	v, cachedError := s.readTranslation(t, rows, target, source)
	if cachedError == nil {
		if e = s.translationSidecars(t, v); e != nil {
			v.Status += " · تعذر تحديث الملف الجانبي"
		}
		s.translationMu.Unlock()
		return v, nil
	}
	s.translationMu.Unlock()
	v = TranslationView{Lines: rows, Target: target, RequestedSource: source, OriginalHash: translationHash(rows), Status: "لم تُترجم الكلمات بهذه اللغة بعد"}
	if automatic {
		if e = s.StartTranslation(id, target, source, false); e != nil {
			v.Status = e.Error()
		} else {
			v.Status = "الترجمة المحلية قيد التجهيز؛ الأصل محفوظ"
		}
	}
	return v, nil
}
func translationRequestKey(r TranslationRequest) string {
	return r.TaskID + "|" + r.Hash + "|" + r.Target + "|" + r.Source
}
func (s *Studio) StartTranslation(id, target, source string, refresh bool) error {
	if !validTranslationLanguage(target, source) {
		return errors.New("اختر لغة مدعومة")
	}
	if !s.translationReady() {
		return errors.New("احتفظ بمحرك الترجمة والنموذج داخل مجلد tools بجانب التطبيق")
	}
	t, ok := s.task(id)
	if !ok {
		return errors.New("المسار غير موجود")
	}
	rows, e := s.translationOriginal(t)
	if e != nil {
		return e
	}
	r := TranslationRequest{TaskID: id, Target: target, Source: source, Hash: translationHash(rows)}
	if !refresh {
		if _, e = s.readTranslation(t, rows, target, source); e == nil {
			return nil
		}
	}
	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		return errors.New("التطبيق قيد الإغلاق")
	}
	if s.translationFailures == nil {
		s.translationFailures = map[string]bool{}
	}
	if !refresh && s.translationFailures[translationRequestKey(r)] {
		s.mu.Unlock()
		return errors.New("تعذرت المحاولة السابقة؛ اختر اللغة الأصلية ثم اضغط إعادة الترجمة")
	}
	for _, q := range s.state.TranslationQueue {
		if q == r {
			s.mu.Unlock()
			return nil
		}
	}
	delete(s.translationFailures, translationRequestKey(r))
	s.state.TranslationQueue = append(s.state.TranslationQueue, r)
	s.save()
	s.mu.Unlock()
	go s.nextTranslation()
	return nil
}
func (s *Studio) nextTranslation() {
	s.mu.Lock()
	if s.closing || s.translationState.Running || len(s.state.TranslationQueue) == 0 {
		s.mu.Unlock()
		return
	}
	r := s.state.TranslationQueue[0]
	ctx := s.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	run, cancel := context.WithTimeout(ctx, 30*time.Minute)
	revision := s.translationState.Revision + 1
	s.translationCancel = cancel
	s.translationState = TranslationState{Running: true, TaskID: r.TaskID, Target: r.Target, Phase: "بدء الترجمة المحلية", Revision: revision}
	s.mu.Unlock()
	e := s.runTranslation(run, r)
	cancel()
	s.mu.Lock()
	s.translationState.Running = false
	s.translationState.Revision++
	s.translationCancel = nil
	if e != nil {
		s.translationState.Error = e.Error()
		s.translationState.Phase = "تعذرت الترجمة؛ الكلمات الأصلية محفوظة"
		if s.translationFailures == nil {
			s.translationFailures = map[string]bool{}
		}
		s.translationFailures[translationRequestKey(r)] = true
	} else {
		s.translationState.Progress = 1
		s.translationState.Phase = "حُفظت الترجمة محليًا"
	}
	if !s.closing && len(s.state.TranslationQueue) > 0 {
		s.state.TranslationQueue = s.state.TranslationQueue[1:]
	}
	s.save()
	s.mu.Unlock()
	go s.nextTranslation()
}
func (s *Studio) CancelTranslation() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.translationCancel != nil {
		s.translationCancel()
	}
}
func (s *Studio) runTranslation(ctx context.Context, r TranslationRequest) error {
	release, slotErr := s.acquireHeavyTask(ctx, "translation")
	if slotErr != nil {
		return slotErr
	}
	defer release()
	t, ok := s.task(r.TaskID)
	if !ok {
		return errors.New("المسار غير موجود")
	}
	rows, e := s.translationOriginal(t)
	if e != nil {
		return e
	}
	if translationHash(rows) != r.Hash {
		return errors.New("تغيرت الكلمات؛ اطلب ترجمة النص الجديد")
	}
	before, _ := s.readTranslation(t, rows, r.Target, r.Source)
	dir := filepath.Join(s.root, "data", "translation-work")
	if e = os.MkdirAll(dir, 0755); e != nil {
		return e
	}
	token := key(translationRequestKey(r))
	input := filepath.Join(dir, token+"-request.json")
	output := filepath.Join(dir, token+"-result.json")
	texts := []string{}
	for _, row := range rows {
		texts = append(texts, row.Original)
	}
	b, _ := json.Marshal(map[string]any{"lines": texts, "target": r.Target, "source": r.Source, "model": filepath.Join(s.root, "tools", "models", "nllb-int8"), "output": output})
	if e = writeChanged(input, b); e != nil {
		return e
	}
	os.Remove(output)
	cmd := hiddenCommand(ctx, filepath.Join(s.root, "tools", "translator", "MadarTranslate.exe"), input)
	cmd.Env = append(os.Environ(), "HF_HUB_OFFLINE=1", "TRANSFORMERS_OFFLINE=1", "TOKENIZERS_PARALLELISM=false")
	stdout, e := cmd.StdoutPipe()
	if e != nil {
		return e
	}
	var stderr limitedBuffer
	stderr.Limit = 8000
	cmd.Stderr = &stderr
	cmd.Env = withWorkerEnvironment(cmd.Env, s.backgroundWorkerEnv())
	if e = cmd.Start(); e != nil {
		return e
	}
	s.applyWorkerBudget(cmd)
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		var u struct {
			Progress float64 `json:"progress"`
			Phase    string  `json:"phase"`
		}
		if json.Unmarshal(scanner.Bytes(), &u) == nil {
			s.mu.Lock()
			s.translationState.Progress = u.Progress
			s.translationState.Phase = u.Phase
			s.mu.Unlock()
		}
	}
	e = cmd.Wait()
	if ctx.Err() != nil {
		return errors.New("أُوقفت الترجمة؛ الأصل محفوظ")
	}
	if e != nil {
		return fmt.Errorf("محرك الترجمة: %s", trimError(stderr.Bytes()))
	}
	b, e = os.ReadFile(output)
	if e != nil {
		return e
	}
	var result struct {
		Source       string   `json:"source"`
		Target       string   `json:"target"`
		Lines        []string `json:"lines"`
		Engine       string   `json:"engine"`
		SameLanguage bool     `json:"sameLanguage"`
	}
	if e = json.Unmarshal(b, &result); e != nil {
		return e
	}
	if len(result.Lines) != len(rows) || result.Target != r.Target || !translationLanguages[result.Source] {
		return errors.New("نتيجة لا تطابق سطور الأصل")
	}
	for i := range rows {
		if strings.TrimSpace(result.Lines[i]) == "" || strings.ContainsAny(result.Lines[i], "\r\n") {
			return errors.New("سطر ترجمة غير صالح")
		}
		rows[i].Text = result.Lines[i]
	}
	s.translationMu.Lock()
	defer s.translationMu.Unlock()
	// Do not attach stale translations if the user edits lyrics during inference.
	latest, ok := s.task(t.ID)
	if !ok {
		return errors.New("أُزيل المسار أثناء الترجمة")
	}
	now, e := s.translationOriginal(latest)
	if e != nil || translationHash(now) != r.Hash {
		return errors.New("تغيرت الكلمات أثناء الترجمة؛ الأصل الجديد محفوظ")
	}
	for i := range rows {
		rows[i].Time = now[i].Time
	}
	if saved, e := s.readTranslation(latest, now, r.Target, r.Source); e == nil && saved.Reviewed && saved.SavedAt != before.SavedAt {
		return errors.New("حُفظ تصحيح أثناء الترجمة؛ احتُفظ بالتصحيح")
	}
	v := TranslationView{Lines: rows, Target: r.Target, Source: result.Source, RequestedSource: r.Source, OriginalHash: r.Hash, Engine: result.Engine, SameLanguage: result.SameLanguage}
	return s.saveTranslation(latest, v)
}
func (s *Studio) SaveTranslation(id, target, source, originalHash string, texts []string) error {
	s.translationMu.Lock()
	defer s.translationMu.Unlock()
	if !validTranslationLanguage(target, source) {
		return errors.New("لغة غير مدعومة")
	}
	t, ok := s.task(id)
	if !ok {
		return errors.New("المسار غير موجود")
	}
	rows, e := s.translationOriginal(t)
	if e != nil {
		return e
	}
	if translationHash(rows) != originalHash || len(texts) != len(rows) {
		return errors.New("تغيرت الكلمات؛ أعد فتح محرر الترجمة")
	}
	v, e := s.readTranslation(t, rows, target, source)
	if e != nil {
		v = TranslationView{Lines: rows, Target: target, RequestedSource: source, OriginalHash: originalHash, Engine: "تصحيح محلي"}
	}
	for i, text := range texts {
		if strings.TrimSpace(text) == "" || strings.ContainsAny(text, "\r\n") || len(text) > 12000 {
			return fmt.Errorf("أدخل ترجمة في السطر %d دون فواصل أسطر", i+1)
		}
		v.Lines[i].Text = strings.TrimSpace(text)
	}
	v.Reviewed = true
	v.SameLanguage = false
	return s.saveTranslation(t, v)
}
