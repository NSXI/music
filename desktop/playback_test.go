package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExtendedFormatsUseLocalDecoder(t *testing.T) {
	s := testStudio(t)
	fixtureAudio(t, s)
	for _, format := range []struct{ Ext, Codec string }{{"m4a", "alac"}, {"ac3", "ac3"}, {"wma", "wmav2"}, {"aiff", "pcm_s16be"}} {
		t.Run(format.Ext, func(t *testing.T) {
			path := filepath.Join(s.root, "format."+format.Ext)
			out, e := s.command(context.Background(), "ffmpeg", "-v", "error", "-nostdin", "-y", "-i", filepath.Join(s.root, "fixture.flac"), "-ar", "48000", "-c:a", format.Codec, path).CombinedOutput()
			if e != nil {
				t.Fatalf("%v %s", e, out)
			}
			n, e := s.importPaths(context.Background(), []string{path})
			if e != nil || n != 1 {
				t.Fatal(e)
			}
			task := s.state.Jobs[len(s.state.Jobs)-1]
			p, e := s.PreparePlayback(task.ID)
			if e != nil || !p.Decoded {
				t.Fatalf("decoder not used %+v %v", p, e)
			}
			if st, e := os.Stat(filepath.Join(s.root, "data", "playback", task.ID+".wav")); e != nil || st.Size() < 1000 {
				t.Fatal("decoded media missing")
			}
		})
	}
}
func TestCooldownSurvivesRestartAndCanCancel(t *testing.T) {
	s := testStudio(t)
	task := testTask(s, "cooldown")
	s.setSourceCooldown("https://media.example/audio", time.Hour)
	r := NewStudio(s.root)
	if len(r.state.Cooldowns) != 1 {
		t.Fatal("provider cooldown was not persisted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e := r.sourceCooldown(ctx, task.ID, "https://media.example/another-audio"); e == nil {
		t.Fatal("cooldown cancellation ignored")
	}
}
