package main

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func validAlignmentFixture() alignmentOutput {
	return alignmentOutput{LRC: fixtureLRC, Original: fixtureLRC, OriginalTimed: true, OriginalSource: "fixture", Report: AlignmentReport{Refined: true, TextMatched: true, CharacterTiming: "visual-within-word", ResolutionSeconds: .02}, Lines: []WordLine{{Start: .1, End: .4, Text: "First test line", Words: []TimedWord{{Text: "First", Start: .1, End: .2, Probability: .9}, {Text: " test", Start: .2, End: .3, Probability: .9}, {Text: " line", Start: .3, End: .4, Probability: .9}}}, {Start: .6, End: .95, Text: "Second test line", Words: []TimedWord{{Text: "Second", Start: .6, End: .7, Probability: .9}, {Text: " test", Start: .7, End: .8, Probability: .9}, {Text: " line", Start: .8, End: .95, Probability: .9}}}}}
}
func TestAlignmentRejectsMalformedWordsAndText(t *testing.T) {
	if e := validateAlignment(validAlignmentFixture(), 1); e != nil {
		t.Fatal(e)
	}
	for _, edit := range []func(*alignmentOutput){func(r *alignmentOutput) { r.Lines[0].Words[0].Start = math.NaN() }, func(r *alignmentOutput) { r.Lines[0].Words[0].End = math.Inf(1) }, func(r *alignmentOutput) { r.Lines[0].Words[0].End = .1 }, func(r *alignmentOutput) { r.Lines[0].Words[1].Start = .05 }, func(r *alignmentOutput) { r.Lines[1].Words[2].End = 2 }, func(r *alignmentOutput) { r.Lines[0].Words[0].Text = "Changed" }, func(r *alignmentOutput) { r.LRC = "[00:00.10]Different text" }, func(r *alignmentOutput) { r.Original = "Different words" }, func(r *alignmentOutput) { r.Lines[0].Words[0].Probability = math.NaN() }} {
		r := validAlignmentFixture()
		edit(&r)
		if validateAlignment(r, 1) == nil {
			t.Fatal("invalid candidate accepted")
		}
	}
	if alignmentIdentity("أغنية  music") != alignmentIdentity("أغنية\nmusic") {
		t.Fatal("Unicode identity lost")
	}
}
func TestAlignmentPreviewRestoreAndStaleText(t *testing.T) {
	s := testStudio(t)
	task := lyricTask(t, s)
	r := validAlignmentFixture()
	r.OriginalWords = r.Lines
	r.OriginalReport = &AlignmentReport{Model: "source-report"}
	if e := s.writeLyricCache(LyricCache{Song: task.Song, Record: LyricsRecord{Synced: fixtureLRC, Provider: "fixture"}, Words: r.OriginalWords}); e != nil {
		t.Fatal(e)
	}
	dir := filepath.Join(s.root, "data", "alignment")
	os.MkdirAll(dir, 0755)
	raw, _ := json.Marshal(r)
	os.WriteFile(filepath.Join(dir, task.ID+"-result.json"), raw, 0600)
	preview, e := s.GetAlignmentPreview(task.ID)
	if e != nil || len(preview.Lines) != 2 || !preview.Report.Refined {
		t.Fatal("word preview lost", e)
	}
	if e = s.ApplyAlignment(task.ID); e != nil {
		t.Fatal(e)
	}
	if e = s.RestoreLyrics(task.ID); e != nil {
		t.Fatal(e)
	}
	cache, e := s.readLyricCache(task.Song)
	if e != nil || len(cache.Words) != 2 || cache.Alignment == nil || cache.Alignment.Model != "source-report" {
		t.Fatal("original word timing not restored", e)
	}
	if e = s.SaveLyricsText(task.ID, "User edited original words", false); e != nil {
		t.Fatal(e)
	}
	if s.ApplyAlignment(task.ID) == nil {
		t.Fatal("old result applied over changed text")
	}
	cache, _ = s.readLyricCache(task.Song)
	if cache.Record.Plain != "User edited original words" {
		t.Fatal("user edit replaced")
	}
}
func TestAlignmentVersionedWorkerSelection(t *testing.T) {
	s := testStudio(t)
	dir := filepath.Join(s.root, "tools", "aligner")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "MadarAlign.exe"), []byte{1}, 0600)
	if filepath.Base(s.alignmentWorker()) != "MadarAlign.exe" {
		t.Fatal("legacy fallback")
	}
	os.WriteFile(filepath.Join(dir, "MadarAlignV2.exe"), []byte{2}, 0600)
	if filepath.Base(s.alignmentWorker()) != "MadarAlignV2.exe" {
		t.Fatal("refinement worker not selected")
	}
}
