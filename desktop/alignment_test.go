package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPackagedLocalAlignment(t *testing.T) {
	if os.Getenv("MADAR_ALIGN") != "1" {
		t.Skip("MADAR_ALIGN=1 enables the packaged offline model test")
	}
	s := testStudio(t)
	source, _ := filepath.Abs("tools")
	if _, e := os.Stat(source); e != nil {
		source, _ = filepath.Abs("../tools")
	}
	dest := filepath.Join(s.root, "tools")
	for _, name := range []string{"aligner", "models"} {
		e := filepath.WalkDir(filepath.Join(source, name), func(path string, d os.DirEntry, e error) error {
			if e != nil {
				return e
			}
			rel, _ := filepath.Rel(source, path)
			target := filepath.Join(dest, rel)
			if d.IsDir() {
				return os.MkdirAll(target, 0755)
			}
			return os.Link(path, target)
		})
		if e != nil {
			t.Fatal(e)
		}
	}
	voice := os.Getenv("MADAR_ALIGNMENT_FIXTURE")
	if voice == "" {
		t.Fatal("fixture directory is missing")
	}
	file := filepath.Join(s.root, "voice.wav")
	if e := copyAtomic(filepath.Join(voice, "voice.wav"), file); e != nil {
		t.Fatal(e)
	}
	s.state.Preferences.AutoAlign = false
	n, e := s.importPaths(context.Background(), []string{file})
	if e != nil || n != 1 {
		t.Fatal(e)
	}
	task := s.state.Jobs[0]
	text, e := os.ReadFile(filepath.Join(voice, "text.txt"))
	if e != nil {
		t.Fatal(e)
	}
	if e = s.SaveLyricsText(task.ID, string(text), false); e != nil {
		t.Fatal(e)
	}
	if e = s.StartAlignment(task.ID, "en"); e != nil {
		t.Fatal(e)
	}
	deadline := time.Now().Add(90 * time.Second)
	var state AlignmentState
	for time.Now().Before(deadline) {
		state = s.GetState().Alignment
		if !state.Running {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if state.Running {
		s.CancelAlignment()
		t.Fatal("alignment deadline exceeded")
	}
	if state.Error != "" || state.Report == nil {
		t.Fatalf("packaged alignment failed %+v", state)
	}
	if state.Report.Coverage < .9 || state.Report.Words < 20 || state.Report.NeedsReview {
		t.Fatalf("unexpected fixture report %+v", state.Report)
	}
	view, e := s.GetLyrics(task.ID, false)
	if e != nil || !view.Timed || len(view.Words) != 5 {
		t.Fatalf("timing not applied/cached: %+v %v", view, e)
	}
	if e = s.RestoreLyrics(task.ID); e != nil {
		t.Fatal(e)
	}
	view, e = s.GetLyrics(task.ID, false)
	if e != nil || view.Timed || view.Text != string(text) {
		t.Fatal("original text did not restore")
	}
	t.Logf("Packaged offline model: %d words, coverage %.0f%%, mean model score %.1f%%", state.Report.Words, state.Report.Coverage*100, state.Report.MeanProbability*100)
}
