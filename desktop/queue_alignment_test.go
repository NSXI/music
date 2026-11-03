package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAutomaticAlignmentDoesNotQueueWhileManualModeIsActive(t *testing.T) {
	s := testStudio(t)
	os.MkdirAll(filepath.Join(s.root, "tools", "aligner"), 0755)
	os.MkdirAll(filepath.Join(s.root, "tools", "models"), 0755)
	os.WriteFile(filepath.Join(s.root, "tools", "aligner", "MadarAlign.exe"), []byte("fixture only"), 0600)
	os.WriteFile(filepath.Join(s.root, "tools", "models", "base.pt"), []byte("fixture only"), 0600)
	s.alignState = AlignmentState{Running: true, TaskID: "active"}
	s.autoAlign("waiting")
	s.autoAlign("waiting")
	s.autoAlign("active")
	if len(s.state.AlignmentQueue) != 0 {
		t.Fatalf("automatic work started in manual mode: %v", s.state.AlignmentQueue)
	}
	r := NewStudio(s.root)
	if len(r.state.AlignmentQueue) != 0 || r.state.Preferences.AutoAlign {
		t.Fatal("automatic alignment was restored after restart")
	}
}
