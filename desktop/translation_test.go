package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTranslationTextIdentityAndTimingRebind(t *testing.T) {
	s := testStudio(t)
	s.state.Preferences.AutoAlign = false
	s.state.Preferences.AutoTranslate = false
	job := lyricTask(t, s)
	original := "[offset:250]\n[00:00.00][00:00.50]The light returns\n[00:00.80]A new day"
	if e := s.SaveLyricsText(job.ID, original, true); e != nil {
		t.Fatal(e)
	}
	rows, _ := s.translationOriginal(job)
	if len(rows) != 3 || *rows[0].Time != .25 || *rows[1].Time != .75 {
		t.Fatal(rows)
	}
	hash := translationHash(rows)
	if e := s.SaveTranslation(job.ID, "ar", "en", hash, []string{"يعود الضوء", "يعود الضوء", "يوم جديد"}); e != nil {
		t.Fatal(e)
	}
	// Same text with new timings must keep reviewed translations and update LRC.
	updated := "[00:00.10]The light returns\n[00:00.60]The light returns\n[00:00.90]A new day"
	if e := s.SaveLyricsText(job.ID, updated, true); e != nil {
		t.Fatal(e)
	}
	v, e := s.GetTranslation(job.ID, "ar", "en", false)
	if e != nil || !v.Exists || !v.Reviewed || *v.Lines[0].Time != .1 {
		t.Fatalf("%+v %v", v, e)
	}
	sidecar := strings.TrimSuffix(job.Path, filepath.Ext(job.Path)) + ".translation.ar.lrc"
	b, e := os.ReadFile(sidecar)
	if e != nil || !strings.Contains(string(b), "[00:00.10]يعود الضوء") {
		t.Fatal(string(b), e)
	}
	// Restart needs neither the model nor network for cached translations.
	restarted := NewStudio(s.root)
	v, e = restarted.GetTranslation(job.ID, "ar", "en", false)
	if e != nil || !v.Exists || !v.Reviewed {
		t.Fatal(v, e)
	}
	if e = s.SaveTranslation(job.ID, "ar", "en", hash, []string{"x"}); e == nil {
		t.Fatal("accepted missing lines")
	}
	if e = s.SaveLyricsText(job.ID, "The text was changed\nA different second line", false); e != nil {
		t.Fatal(e)
	}
	v, e = s.GetTranslation(job.ID, "ar", "en", false)
	if e != nil || v.Exists {
		t.Fatal("stale translation attached", v, e)
	}
	if e = s.SaveTranslation(job.ID, "ar", "en", hash, []string{"x", "y"}); e == nil {
		t.Fatal("accepted stale editor")
	}
	if _, e = s.GetTranslation(job.ID, "../../other", "en", false); e == nil {
		t.Fatal("accepted invalid language")
	}
}
func TestTranslationCacheValidation(t *testing.T) {
	s := testStudio(t)
	s.state.Preferences.AutoAlign = false
	s.state.Preferences.AutoTranslate = false
	job := lyricTask(t, s)
	s.SaveLyricsText(job.ID, "First line\nSecond line", false)
	rows, _ := s.translationOriginal(job)
	v := TranslationView{Lines: rows, OriginalHash: translationHash(rows), Target: "ar", RequestedSource: "en"}
	v.Lines[0].Text = "أول"
	v.Lines[1].Text = "ثان"
	if e := s.saveTranslation(job, v); e != nil {
		t.Fatal(e)
	}
	v.Lines[1].Original = "Different text"
	s.saveTranslation(job, v)
	if _, e := s.readTranslation(job, rows, "ar", "en"); e == nil {
		t.Fatal("corrupt cache attached")
	}
}

func TestTranslationQueuePersistsAndDeduplicatesLanguages(t *testing.T) {
	s := testStudio(t)
	s.state.Preferences.AutoAlign = false
	s.state.Preferences.AutoTranslate = false
	job := lyricTask(t, s)
	if e := s.SaveLyricsText(job.ID, "First original line\nSecond original line", false); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"tools/translator/MadarTranslate.exe", "tools/models/nllb-int8/model.bin", "tools/models/nllb-int8/tokenizer.json"} {
		path := filepath.Join(s.root, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(path), 0755)
		os.WriteFile(path, []byte("fixture, never executed"), 0600)
	}
	s.translationState.Running = true // Hold the controlled queue without starting a worker.
	for _, language := range []string{"ar", "ar", "fr"} {
		if e := s.StartTranslation(job.ID, language, "en", false); e != nil {
			t.Fatal(e)
		}
	}
	state := s.GetState()
	if len(state.TranslationQueue) != 2 {
		t.Fatal(state.TranslationQueue)
	}
	restarted := NewStudio(s.root)
	if len(restarted.GetState().TranslationQueue) != 2 {
		t.Fatal("translation queue was lost on restart")
	}
}
func TestPackagedOfflineTranslation(t *testing.T) {
	if os.Getenv("MADAR_TRANSLATE") != "1" {
		t.Skip("MADAR_TRANSLATE=1 enables the actual local multilingual model")
	}
	s := testStudio(t)
	s.state.Preferences.AutoAlign = false
	s.state.Preferences.AutoTranslate = false
	source, _ := filepath.Abs("tools")
	if _, e := os.Stat(source); e != nil {
		source, _ = filepath.Abs("../tools")
	}
	for _, name := range []string{"translator", "models/nllb-int8"} {
		e := filepath.WalkDir(filepath.Join(source, filepath.FromSlash(name)), func(path string, d os.DirEntry, e error) error {
			if e != nil {
				return e
			}
			rel, _ := filepath.Rel(source, path)
			target := filepath.Join(s.root, "tools", rel)
			if d.IsDir() {
				return os.MkdirAll(target, 0755)
			}
			return os.Link(path, target)
		})
		if e != nil {
			t.Fatal(e)
		}
	}
	job := lyricTask(t, s)
	text := "[00:00.00]The light returns with every morning.\n[00:00.50]I carry this melody in my heart."
	if e := s.SaveLyricsText(job.ID, text, true); e != nil {
		t.Fatal(e)
	}
	for _, target := range []string{"ar", "fr", "ja"} {
		if e := s.StartTranslation(job.ID, target, "en", false); e != nil {
			t.Fatal(e)
		}
	}
	// Queue deduplication and serialization include language, not just song ID.
	if e := s.StartTranslation(job.ID, "ar", "en", false); e != nil {
		t.Fatal(e)
	}
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		state := s.GetState()
		if !state.Translation.Running && len(state.TranslationQueue) == 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	state := s.GetState()
	if state.Translation.Running || state.Translation.Error != "" {
		t.Fatal(state.Translation)
	}
	for _, target := range []string{"ar", "fr", "ja"} {
		v, e := s.GetTranslation(job.ID, target, "en", false)
		if e != nil || !v.Exists || len(v.Lines) != 2 || v.Source != "en" || v.Reviewed {
			t.Fatal(target, v, e)
		}
		if strings.TrimSpace(v.Lines[0].Text) == "" || v.Lines[0].Text == v.Lines[0].Original {
			t.Fatal("missing actual translation", target, v)
		}
		t.Logf("%s: %s", target, v.Lines[0].Text)
	}
	restarted := NewStudio(s.root)
	v, e := restarted.GetTranslation(job.ID, "ar", "en", false)
	if e != nil || !v.Exists {
		t.Fatal(v, e)
	}
}
