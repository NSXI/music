package main

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"sync"
)

type ResourceState struct {
	Mode               string `json:"mode"`
	ActiveHeavy        string `json:"activeHeavy,omitempty"`
	RequestedCores     int    `json:"requestedCores"`
	AffinityApplied    bool   `json:"affinityApplied"`
	LowPriorityApplied bool   `json:"lowPriorityApplied"`
}

func normalizeResourceMode(mode string) string {
	if mode == "balanced" || mode == "smooth" {
		return mode
	}
	return "economy"
}
func workerCores(mode string) int {
	switch normalizeResourceMode(mode) {
	case "balanced":
		return 4
	case "smooth":
		return 6
	default:
		return 2
	}
}
func (s *Studio) SetResourceMode(mode string) error {
	if mode != "economy" && mode != "balanced" && mode != "smooth" {
		return errors.New("وضع موارد غير صالح")
	}
	s.mu.Lock()
	s.state.Preferences.ResourceMode = mode
	s.resourceState.Mode = mode
	s.save()
	s.mu.Unlock()
	return nil
}
func (s *Studio) acquireHeavyTask(ctx context.Context, kind string) (func(), error) {
	s.mu.Lock()
	if s.analysisSlot == nil {
		s.analysisSlot = make(chan struct{}, 1)
	}
	slot := s.analysisSlot
	s.mu.Unlock()
	select {
	case slot <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	s.mu.Lock()
	s.resourceState.ActiveHeavy = kind
	s.resourceState.Mode = normalizeResourceMode(s.state.Preferences.ResourceMode)
	s.mu.Unlock()
	var once sync.Once
	return func() { once.Do(func() { s.mu.Lock(); s.resourceState.ActiveHeavy = ""; s.mu.Unlock(); <-slot }) }, nil
}

func (s *Studio) applyWorkerBudget(cmd *exec.Cmd) ResourceState {s.mu.Lock();defer s.mu.Unlock();state:=ResourceState{Mode:normalizeResourceMode(s.state.Preferences.ResourceMode),RequestedCores:workerCores(s.state.Preferences.ResourceMode),ActiveHeavy:s.resourceState.ActiveHeavy};s.resourceState=state;return state}
func (s *Studio) backgroundWorkerEnv() []string {
	s.mu.Lock()
	cores := workerCores(s.state.Preferences.ResourceMode)
	s.mu.Unlock()
	value := "2"
	if cores == 4 {
		value = "4"
	} else if cores == 6 {
		value = "6"
	}
	return []string{"OMP_NUM_THREADS=" + value, "MKL_NUM_THREADS=" + value, "OPENBLAS_NUM_THREADS=" + value, "VECLIB_MAXIMUM_THREADS=" + value, "NUMEXPR_NUM_THREADS=" + value, "TOKENIZERS_PARALLELISM=false"}
}
func withWorkerEnvironment(existing, extra []string) []string {
	keys := map[string]bool{}
	for _, entry := range extra {
		keys[strings.SplitN(entry, "=", 2)[0]] = true
	}
	out := []string{}
	for _, entry := range existing {
		if !keys[strings.SplitN(entry, "=", 2)[0]] {
			out = append(out, entry)
		}
	}
	return append(out, extra...)
}
