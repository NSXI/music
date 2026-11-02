package main

import (
	"context"
	"net/url"
	"time"
)

func (s *Studio) sourceCooldown(ctx context.Context, id, address string) error {
	u, e := url.Parse(address)
	if e != nil {
		return e
	}
	s.mu.Lock()
	deadline := s.state.Cooldowns[u.Host]
	s.mu.Unlock()
	wait := time.Until(time.UnixMilli(deadline))
	if wait > 0 {
		return s.waitRetry(ctx, id, wait)
	}
	return nil
}
func (s *Studio) setSourceCooldown(address string, wait time.Duration) {
	u, e := url.Parse(address)
	if e != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.Cooldowns == nil {
		s.state.Cooldowns = map[string]int64{}
	}
	deadline := time.Now().Add(wait).UnixMilli()
	if deadline > s.state.Cooldowns[u.Host] {
		s.state.Cooldowns[u.Host] = deadline
	}
	s.save()
}
