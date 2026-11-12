package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type MigrationReport struct {
	BackupCreated bool     `json:"backupCreated"`
	At            string   `json:"at,omitempty"`
	TracksBefore  int      `json:"tracksBefore"`
	Tracks        int      `json:"tracks"`
	NeedsRelink   []string `json:"needsRelink"`
}

func (s *Studio) prepareV3Migration() {
	dir := filepath.Join(s.root, "data", "backups")
	marker := filepath.Join(dir, "v3-migration.json")
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		return
	}
	library := filepath.Join(s.root, "data", "library.json")
	if _, err := os.Stat(library); err != nil {
		return
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return
	}
	if err := copyAtomic(library, filepath.Join(dir, "library-before-v3.json")); err != nil {
		return
	}
	if _, err := os.Stat(s.workspacePath()); err == nil {
		if err = copyAtomic(s.workspacePath(), filepath.Join(dir, "workspace-before-v3.json")); err != nil {
			return
		}
	}
	report := MigrationReport{BackupCreated: true, At: time.Now().UTC().Format(time.RFC3339)}
	for _, task := range s.GetState().Jobs {
		if task.Status == "done" && task.UpgradeFor == "" {
			report.TracksBefore++
		}
	}
	raw, _ := json.Marshal(report)
	_ = writeChanged(marker, raw)
}

func (s *Studio) GetMigrationReport() MigrationReport {
	var result MigrationReport
	marker := filepath.Join(s.root, "data", "backups", "v3-migration.json")
	if data, err := os.ReadFile(marker); err == nil {
		_ = json.Unmarshal(data, &result)
	}
	result.NeedsRelink = []string{}
	for _, task := range s.GetState().Jobs {
		if task.Status != "done" || task.UpgradeFor != "" {
			continue
		}
		result.Tracks++
		if _, err := os.Stat(task.Path); err != nil {
			result.NeedsRelink = append(result.NeedsRelink, task.Song.Title)
		}
	}
	return result
}
