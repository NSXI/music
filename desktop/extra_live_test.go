package main

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestExtraLiveSearchAndTransfer(t *testing.T) {
	if os.Getenv("MADAR_EXTRA_LIVE") != "1" {
		t.Skip("MADAR_EXTRA_LIVE=1 enables public additional provider checks")
	}
	s := testStudio(t)
	s.state.Preferences.AutoMatch = false
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	for _, provider := range []string{"audius", "hearthis", "mixcloud", "commons", "ytmusic"} {
		r, e := s.providerSearch(ctx, provider, "piano")
		if e != nil {
			t.Errorf("%s search: %v", provider, e)
			continue
		}
		if len(r.Tracks) == 0 {
			t.Errorf("%s returned no results", provider)
			continue
		}
		s.remember(r)
		t.Logf("%s metadata: %d results; artwork=%t", provider, len(r.Tracks), r.Tracks[0].Artwork != "")
		if provider != "audius" && provider != "commons" {
			continue
		}
		song := r.Tracks[0]
		song.OriginalURL = "" // Verify the public stream, not a gated original.
		job := testTask(s, "live-extra-"+provider)
		job.Song = song
		s.state.Jobs[len(s.state.Jobs)-1] = job
		path, e := s.fetchCandidate(ctx, job, song, s.GetState().Preferences)
		if e != nil {
			t.Errorf("%s transfer: %v", provider, e)
			continue
		}
		quality, e := s.probe(ctx, path)
		if e == nil {
			e = s.verify(ctx, path)
		}
		if e != nil {
			t.Errorf("%s decode: %v", provider, e)
			continue
		}
		t.Logf("%s actual audio: %s %d Hz %.1f sec", provider, quality.Codec, quality.Rate, quality.Duration)
	}
}

func TestIndexedLiveProviders(t *testing.T) {
	if os.Getenv("MADAR_EXTRA_LIVE") != "1" {
		t.Skip("Public indexed provider availability")
	}
	for _, provider := range []string{"jamendo", "audiomack", "reverbnation"} {
		t.Run(provider, func(t *testing.T) {
			s := testStudio(t)
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
			defer cancel()
			r, e := s.providerSearch(ctx, provider, "piano")
			if e != nil || len(r.Tracks) == 0 {
				t.Skipf("Live source unavailable/no indexed result: %v; results=%d", e, len(r.Tracks))
			}
			t.Logf("Actual indexed metadata: %d tracks; artwork=%t", len(r.Tracks), r.Tracks[0].Artwork != "")
		})
	}
}
