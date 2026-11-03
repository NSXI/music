package main

import (
	"context"
	"time"
)

type lyricFetchResult struct {
	record LyricsRecord
	err    error
}

func (s *Studio) prefetchWordLyrics(ctx context.Context, song Song, providers []string) map[string]<-chan lyricFetchResult {
	results := map[string]<-chan lyricFetchResult{}
	for _, id := range providers {
		if !wordLyricProvider(id) {
			continue
		}
		channel := make(chan lyricFetchResult, 1)
		results[id] = channel
		go func(provider string) {
			run, cancel := context.WithTimeout(ctx, 12*time.Second)
			defer cancel()
			record, err := s.fetchLyricProvider(run, song, provider)
			channel <- lyricFetchResult{record, err}
		}(id)
	}
	return results
}

// Only sources enabled by the user participate. Prefer actual word timing over
// line timestamps; a provider name alone never establishes the result quality.
func preferredLyricProviders(providers []string) []string {
	order := []string{}
	seen := map[string]bool{}
	for _, id := range []string{"amll", "betterlyrics", "netease"} {
		for _, enabled := range providers {
			if id == enabled && !seen[id] {
				order = append(order, id)
				seen[id] = true
			}
		}
	}
	for _, id := range providers {
		if !seen[id] {
			order = append(order, id)
			seen[id] = true
		}
	}
	return order
}
func wordLyricProvider(id string) bool {
	return id == "amll" || id == "betterlyrics" || id == "netease"
}
func lyricRecordRank(record LyricsRecord, song Song) int {
	if validLRC(record.Synced, song.Duration) {
		if wordTimingComplete(record.Words, record.Synced, song.Duration) {
			return 4
		}
		for _, line := range record.Words {
			if len(line.Words) > 0 {
				return 3
			}
		}
		return 2
	}
	if record.Plain != "" {
		return 1
	}
	if record.Instrumental {
		return 1
	}
	return 0
}
