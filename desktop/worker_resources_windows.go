package main

import (
	"context"
	"errors"
	"golang.org/x/sys/windows"
	"os/exec"
	"strings"
	"sync"
	"unsafe"
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
func boundedAffinityMask(allowed uintptr, limit int) uintptr {
	var out uintptr
	for bit := uint(0); bit < uint(unsafe.Sizeof(allowed)*8) && limit > 0; bit++ {
		flag := uintptr(1) << bit
		if allowed&flag != 0 {
			out |= flag
			limit--
		}
	}
	return out
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

var kernelResource = windows.NewLazySystemDLL("kernel32.dll")
var getResourceAffinity = kernelResource.NewProc("GetProcessAffinityMask")
var setResourceAffinity = kernelResource.NewProc("SetProcessAffinityMask")

func (s *Studio) applyWorkerBudget(cmd *exec.Cmd) ResourceState {
	s.mu.Lock()
	mode := normalizeResourceMode(s.state.Preferences.ResourceMode)
	s.mu.Unlock()
	state := ResourceState{Mode: mode, RequestedCores: workerCores(mode)}
	if cmd == nil || cmd.Process == nil {
		return state
	}
	handle, err := windows.OpenProcess(windows.PROCESS_SET_INFORMATION|windows.PROCESS_QUERY_INFORMATION, false, uint32(cmd.Process.Pid))
	if err != nil {
		return state
	}
	defer windows.CloseHandle(handle)
	state.LowPriorityApplied = windows.SetPriorityClass(handle, windows.BELOW_NORMAL_PRIORITY_CLASS) == nil
	var allowed, system uintptr
	ok, _, _ := getResourceAffinity.Call(uintptr(handle), uintptr(unsafe.Pointer(&allowed)), uintptr(unsafe.Pointer(&system)))
	if ok != 0 {
		mask := boundedAffinityMask(allowed&system, state.RequestedCores)
		if mask != 0 {
			ok, _, _ = setResourceAffinity.Call(uintptr(handle), mask)
			state.AffinityApplied = ok != 0
		}
	}
	s.mu.Lock()
	state.ActiveHeavy = s.resourceState.ActiveHeavy
	s.resourceState = state
	s.mu.Unlock()
	return state
}
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
