package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

func (s *Studio) SetTransferNetworkState(wifi bool) {
	s.mu.Lock()
	s.v2NetworkWiFi = wifi
	s.mu.Unlock()
	if !s.downloadPolicyAllows(time.Now()) {
		s.mu.Lock()
		for _, active := range s.activeDownloads {
			if active.action == "" {
				active.action = "policy"
				active.cancel()
			}
		}
		s.mu.Unlock()
	}
	s.signal()
}

func (s *Studio) enforceDownloadPolicy() {
	if !s.downloadPolicyAllows(time.Now()) {
		s.mu.Lock()
		for _, active := range s.activeDownloads {
			if active.action == "" {
				active.action = "policy"
				active.cancel()
			}
		}
		s.mu.Unlock()
	}
	s.signal()
}
func (s *Studio) downloadPolicyAllows(at time.Time) bool {
	if runtime.GOOS == "android" {
		s.mu.Lock()
		held := s.v2SchedulerHold
		s.mu.Unlock()
		if held {
			return false
		}
	}
	p := s.downloadPolicy()
	if !downloadAllowedAt(p, s.transferLocalTime(at)) {
		return false
	}
	return s.transferNetworkAllowed(p)
}
func (s *Studio) prepareV2Migration() {
	// Keep the original library on the first v2 startup, never a migration fixture.
	marker := filepath.Join(s.root, "data", "backups", "v2-migration.json")
	if _, e := os.Stat(marker); os.IsNotExist(e) {
		source := filepath.Join(s.root, "data", "library.json")
		if _, e = os.Stat(source); e == nil {
			os.MkdirAll(filepath.Dir(marker), 0700)
			if copyAtomic(source, filepath.Join(filepath.Dir(marker), "library-before-v2.json")) == nil {
				writeChanged(marker, []byte(`{"schema":2,"libraryPreserved":true}`))
			}
		}
	}
}

func (s *Studio) v2Maintenance(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	ticks := 0
	for {
		select {
		case <-ctx.Done():
			s.StopDeviceLink()
			s.CancelDeviceTransfer()
			return
		case <-ticker.C:
			s.signal()
			if !s.downloadPolicyAllows(time.Now()) {
				s.mu.Lock()
				for _, active := range s.activeDownloads {
					if active.action == "" {
						active.action = "policy"
						active.cancel()
					}
				}
				s.mu.Unlock()
			}
			ticks++
			if ticks%3 != 0 || runtime.GOOS == "android" {
				continue
			}
			w, e := s.GetWorkspace()
			if e != nil {
				continue
			}
			var watch bool
			json.Unmarshal(w.UI["watchFolders"], &watch)
			if !watch {
				continue
			}
			s.mu.Lock()
			busy := s.folderImport.Running
			s.mu.Unlock()
			if !busy {
				s.RescanLibraryFolders()
			}
		}
	}
}
