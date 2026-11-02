package main

import (
	"errors"
	"regexp"
	"strings"
)

// Provider credentials share the existing per-Windows-user DPAPI store.
// Snapshot and frontend state only receive configured/not-configured flags.
func (s *Studio) metadataKey(provider string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.metadataKeys[provider]
}
func (s *Studio) metadataBase(provider, fallback string) string {
	if base := s.sourceTestBases[provider]; base != "" {
		return strings.TrimRight(base, "/")
	}
	if provider == "lrcapi" {
		s.mu.Lock()
		configured := s.state.Preferences.LrcAPIURL
		s.mu.Unlock()
		if configured != "" {
			return strings.TrimRight(configured, "/")
		}
	}
	return strings.TrimRight(fallback, "/")
}
func (s *Studio) SetMetadataKey(provider, value string) error {
	if !regexp.MustCompile(`^[a-z][a-z0-9-]{1,31}$`).MatchString(provider) || len(value) > 4096 {
		return errors.New("مصدر أو مفتاح غير صالح")
	}
	value = strings.TrimSpace(value)
	s.mu.Lock()
	if s.metadataKeys == nil {
		s.metadataKeys = map[string]string{}
	}
	previous := s.metadataKeys[provider]
	if value == "" {
		delete(s.metadataKeys, provider)
	} else {
		s.metadataKeys[provider] = value
	}
	s.mu.Unlock()
	if err := s.saveAuth(); err != nil {
		s.mu.Lock()
		if previous == "" {
			delete(s.metadataKeys, provider)
		} else {
			s.metadataKeys[provider] = previous
		}
		s.mu.Unlock()
		return errors.New("تعذر حفظ المفتاح مشفرًا")
	}
	return nil
}
