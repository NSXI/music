package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAutomaticAlignmentQueueIsSavedAndDeduplicated(t *testing.T) {
	s := testStudio(t)
	os.MkdirAll(filepath.Join(s.root, "tools", "aligner"), 0755)
	os.MkdirAll(filepath.Join(s.root, "tools", "models"), 0755)
	os.WriteFile(filepath.Join(s.root, "tools", "aligner", "MadarAlign.exe"), []byte("fixture only"), 0600)
	os.WriteFile(filepath.Join(s.root, "tools", "models", "base.pt"), []byte("fixture only"), 0600)
	s.alignState = AlignmentState{Running: true, TaskID: "active"}
	s.autoAlign("waiting")
	s.autoAlign("waiting")
	s.autoAlign("active")
	if len(s.state.AlignmentQueue) != 1 {
		t.Fatalf("automatic queue duplicates: %v", s.state.AlignmentQueue)
	}
	r := NewStudio(s.root)
	if len(r.state.AlignmentQueue) != 1 || r.state.AlignmentQueue[0] != "waiting" {
		t.Fatal("alignment queue was not saved")
	}
}
