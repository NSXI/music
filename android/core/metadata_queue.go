package main

import (
	"context"
	"time"
)

// Artwork, lyrics and tags enrich an already verified playable file. They must
// not hold a download/import open for minutes when a remote service is slow.
func (s *Studio) queueMetadata(id string) {
	s.mu.Lock()
	for i := range s.state.Jobs {
		if s.state.Jobs[i].ID == id {
			s.state.Jobs[i].MetadataPending = true
			break
		}
	}
	s.save()
	s.mu.Unlock()
	select {
	case s.metadataWake <- struct{}{}:
	default:
	}
}
func (s *Studio) metadataLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.metadataWake:
		}
		for {
			if ctx.Err() != nil {
				return
			}
			id := ""
			s.mu.Lock()
			for _, job := range s.state.Jobs {
				if job.MetadataPending && job.Path != "" && job.Status != "cancelled" {
					id = job.ID
					break
				}
			}
			s.mu.Unlock()
			if id == "" {
				break
			}
			run, cancel := context.WithTimeout(ctx, 60*time.Second)
			s.enrich(run, id)
			s.refreshJobGenres(run, id)
			cancel()
			// Keep pending work on shutdown so the next launch can resume it.
			if ctx.Err() != nil {
				return
			}
			s.mu.Lock()
			for i := range s.state.Jobs {
				if s.state.Jobs[i].ID == id {
					s.state.Jobs[i].MetadataPending = false
					s.state.Jobs[i].Phase = ""
					break
				}
			}
			s.save()
			s.mu.Unlock()
		}
	}
}
