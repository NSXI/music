package main

import (
	"os"
	"path/filepath"
	"runtime"
)

type PerformanceSnapshot struct {
	HeapBytes    uint64 `json:"heapBytes"`
	HeapReserved uint64 `json:"heapReserved"`
	Goroutines   int    `json:"goroutines"`
	CPUs         int    `json:"cpus"`
	Tracks       int    `json:"tracks"`
}

func (s *Studio) GetPerformanceSnapshot() PerformanceSnapshot {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	s.mu.Lock()
	n := len(s.state.Jobs)
	s.mu.Unlock()
	return PerformanceSnapshot{m.HeapAlloc, m.HeapSys, runtime.NumGoroutine(), runtime.NumCPU(), n}
}
func (s *Studio) ClearDerivedCaches() (int, error) {
	n := 0
	for _, name := range []string{"waveforms", filepath.Join("artwork", "thumbnails")} {
		dir := filepath.Join(s.root, "data", name)
		entries, e := os.ReadDir(dir)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return n, e
		}
		for _, entry := range entries {
			if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			if e = os.Remove(filepath.Join(dir, entry.Name())); e != nil {
				return n, e
			}
			n++
		}
	}
	return n, nil
}
