package main

import (
	"runtime"
	"time"
)

func (s *Studio) SetTransferTimezoneOffset(seconds int) {
	if seconds < -14*3600 || seconds > 14*3600 {
		return
	}
	s.mu.Lock()
	s.v2TimezoneOffset = seconds
	s.v2TimezoneKnown = true
	s.mu.Unlock()
	s.enforceDownloadPolicy()
}
func (s *Studio) transferLocalTime(at time.Time) time.Time {
	s.mu.Lock()
	offset, known := s.v2TimezoneOffset, s.v2TimezoneKnown
	s.mu.Unlock()
	if known {
		return at.In(time.FixedZone("device", offset))
	}
	return at
}
func (s *Studio) transferNetworkAllowed(p DownloadPolicy) bool {
	if runtime.GOOS != "android" || !p.WiFiOnly {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.v2NetworkWiFi
}
func (s *Studio) GetTransferStatus() map[string]any {
	p := s.downloadPolicy()
	at := s.transferLocalTime(time.Now())
	network := s.transferNetworkAllowed(p)
	allowed := downloadAllowedAt(p, at) && network
	s.mu.Lock()
	held := s.v2SchedulerHold
	queued, working, metadata := 0, 0, 0
	for _, j := range s.state.Jobs {
		if j.Status == "queued" {
			queued++
		}
		if j.Status == "working" {
			working++
		}
		if j.MetadataPending && j.Status != "cancelled" {
			metadata++
		}
	}
	s.mu.Unlock()
	return map[string]any{"queued": queued, "working": working, "metadata": metadata, "held": held, "allowed": allowed, "networkAllowed": network, "readyNow": working > 0 || queued > 0 && allowed || metadata > 0 && network, "hour": at.Hour(), "policy": p}
}
func (s *Studio) schedulerPausedLocked() bool { return runtime.GOOS == "android" && s.v2SchedulerHold }
func (s *Studio) backgroundWorkAllowed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.schedulerPausedLocked()
}
func (s *Studio) HoldTransferQueue() {
	s.mu.Lock()
	s.v2SchedulerHold = true
	for _, active := range s.activeDownloads {
		if active.action == "" {
			active.action = "policy"
			active.cancel()
		}
	}
	if s.schedulerPausedLocked() && s.translationCancel != nil {
		s.v2TranslationSuspended = true
		s.translationCancel()
	}
	s.mu.Unlock()
}
func (s *Studio) WakeTransferQueue() {
	s.mu.Lock()
	s.v2SchedulerHold = false
	s.mu.Unlock()
	s.signal()
	select {
	case s.metadataWake <- struct{}{}:
	default:
	}
	go s.nextTranslation()
}
