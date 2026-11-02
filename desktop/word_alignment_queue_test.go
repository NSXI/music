package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func fakeAlignReady(t *testing.T, s *Studio) {
	t.Helper()
	for _, p := range []string{"tools/aligner/MadarAlignV2.exe", "tools/models/base.pt"} {
		p = filepath.Join(s.root, filepath.FromSlash(p))
		os.MkdirAll(filepath.Dir(p), 0755)
		os.WriteFile(p, []byte{1}, 0600)
	}
	s.alignState.Running = true
	s.state.Preferences.AutoAlign = true
}
func TestStripLyricTextRepeatedTagsAndOrder(t *testing.T) {
	input := "[ar:Original fixture]\n[offset:250]\n[00:01.00][00:09.00]Chorus returns\n[00:04.00]Middle verse"
	want := "Chorus returns\nMiddle verse\nChorus returns"
	if got := stripLyricText(input); got != want {
		t.Fatalf("repeated chorus missing or reordered: %q", got)
	}
	for _, text := range []string{"One plain line\nAnother plain line", "[00:01.00]First line\n[00:02.00]Next line"} {
		if got := stripLyricText(text); got == "" {
			t.Fatal("lost ordinary transcript")
		}
	}
	if got := stripLyricText("[00:02.00]ثاني سطر\n[00:01.00]أول سطر"); got != "أول سطر\nثاني سطر" {
		t.Fatal(got)
	}
	if got := stripLyricText("[00:01.00]<00:01.00>Timed <00:01.50>word\n[00:03.00]Final line"); got != "Timed word\nFinal line" {
		t.Fatal(got)
	}
}
func TestTimedLinesAndLocalSidecarQueueWordAlignment(t *testing.T) {
	for _, local := range []bool{false, true} {
		s := testStudio(t)
		task := lyricTask(t, s)
		fakeAlignReady(t, s)
		if local {
			p := filepath.Join(s.root, "song.lrc")
			os.WriteFile(p, []byte(fixtureLRC), 0600)
			s.state.Jobs[0].Lyrics = p
		} else {
			if e := s.writeLyricCache(LyricCache{Song: task.Song, Record: LyricsRecord{Synced: fixtureLRC, Provider: "lrclib"}}); e != nil {
				t.Fatal(e)
			}
		}
		for i := 0; i < 2; i++ {
			v, e := s.GetLyrics(task.ID, false)
			if e != nil || !v.Timed {
				t.Fatal(v, e)
			}
		}
		if len(s.state.AlignmentQueue) != 1 || s.state.AlignmentQueue[0] != task.ID {
			t.Fatal("line-only lyrics were not queued once", local, s.state.AlignmentQueue)
		}
	}
}
func TestCompleteWordTimesSkipAutomaticAlignment(t *testing.T) {
	s := testStudio(t)
	task := lyricTask(t, s)
	fakeAlignReady(t, s)
	r := validAlignmentFixture()
	if !wordTimingComplete(r.Lines, fixtureLRC, 1) {
		t.Fatal("valid source words")
	}
	if wordTimingComplete(r.Lines[:1], fixtureLRC, 1) {
		t.Fatal("partial words counted complete")
	}
	s.writeLyricCache(LyricCache{Song: task.Song, Record: LyricsRecord{Synced: fixtureLRC}, Words: r.Lines})
	s.GetLyrics(task.ID, false)
	if len(s.state.AlignmentQueue) != 0 {
		t.Fatal("already timed source queued")
	}
}
func TestDisabledAutoAlignLeavesTimedLyricsAlone(t *testing.T) {
	for _, local := range []bool{false, true} {
		s := testStudio(t)
		task := lyricTask(t, s)
		fakeAlignReady(t, s)
		s.state.Preferences.AutoAlign = false
		if local {
			p := filepath.Join(s.root, "song.lrc")
			if e := os.WriteFile(p, []byte(fixtureLRC), 0600); e != nil {
				t.Fatal(e)
			}
			s.state.Jobs[0].Lyrics = p
		} else if e := s.writeLyricCache(LyricCache{Song: task.Song, Record: LyricsRecord{Synced: fixtureLRC}}); e != nil {
			t.Fatal(e)
		}
		if _, e := s.GetLyrics(task.ID, false); e != nil {
			t.Fatal(e)
		}
		if len(s.state.AlignmentQueue) != 0 {
			t.Fatal("disabled auto alignment queued work", local)
		}
	}
}
func TestLegacyAlignmentUpgradeAndChangedText(t *testing.T) {
	s := testStudio(t)
	task := lyricTask(t, s)
	fakeAlignReady(t, s)
	s.writeLyricCache(LyricCache{Song: task.Song, Record: LyricsRecord{Synced: fixtureLRC}})
	dir := filepath.Join(s.root, "data", "alignment")
	os.MkdirAll(dir, 0755)
	p := filepath.Join(dir, task.ID+"-result.json")
	r := validAlignmentFixture()
	r.Report.Refined = false
	r.Report.Model = "legacy"
	raw, _ := json.Marshal(r)
	os.WriteFile(p, raw, 0600)
	s.autoAlign(task.ID)
	if len(s.state.AlignmentQueue) != 1 {
		t.Fatal("legacy not upgraded")
	}
	s.state.AlignmentQueue = nil
	r.Report.Refined = true
	raw, _ = json.Marshal(r)
	os.WriteFile(p, raw, 0600)
	s.autoAlign(task.ID)
	if len(s.state.AlignmentQueue) != 0 {
		t.Fatal("same completed refinement repeated")
	}
	broken := r
	broken.Lines = nil
	raw, _ = json.Marshal(broken)
	os.WriteFile(p, raw, 0600)
	s.autoAlign(task.ID)
	if len(s.state.AlignmentQueue) != 1 {
		t.Fatal("invalid completed candidate blocked repair")
	}
	s.state.AlignmentQueue = nil
	raw, _ = json.Marshal(r)
	os.WriteFile(p, raw, 0600)
	s.writeLyricCache(LyricCache{Song: task.Song, Record: LyricsRecord{Plain: "New user text"}})
	s.autoAlign(task.ID)
	if len(s.state.AlignmentQueue) != 1 {
		t.Fatal("changed text skipped")
	}
}
