package main

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type AlbumLevel struct {
	LoudnessInfo
	Measured int `json:"measured"`
	Tracks   int `json:"tracks"`
}

func (s *Studio) AnalyzeAlbumLoudness(id string) (AlbumLevel, error) {
	task, ok := s.task(id)
	if !ok || task.Status != "done" || strings.TrimSpace(task.Song.Album) == "" {
		return AlbumLevel{}, errors.New("اختر أغنية محفوظة ضمن ألبوم")
	}
	album, artist := norm(task.Song.Album), norm(task.Song.Artist)
	var tracks []Task
	var fingerprint []string
	for _, candidate := range s.GetState().Jobs {
		if candidate.Status != "done" || candidate.UpgradeFor != "" ||
			norm(candidate.Song.Album) != album || norm(candidate.Song.Artist) != artist {
			continue
		}
		tracks = append(tracks, candidate)
		fingerprint = append(fingerprint, candidate.ID+":"+candidate.Hash+":"+candidate.Path)
	}
	if len(tracks) == 0 || len(tracks) > 100 {
		return AlbumLevel{}, errors.New("تحليل الألبوم يدعم حتى 100 مسار")
	}
	sort.Strings(fingerprint)
	path := filepath.Join(s.root, "data", "loudness", "album-"+key(strings.Join(fingerprint, "\n"))+".json")
	var result AlbumLevel
	if raw, err := os.ReadFile(path); err == nil && json.Unmarshal(raw, &result) == nil && result.Measured > 0 {
		return result, nil
	}
	result.Tracks = len(tracks)
	energy, seconds, peak := 0.0, 0.0, -120.0
	for _, track := range tracks {
		level, err := s.AnalyzeLoudness(track.ID)
		if err != nil {
			continue
		}
		length := max(1, track.Song.Duration)
		energy += math.Pow(10, level.IntegratedLUFS/10) * length
		seconds += length
		peak = max(peak, level.TruePeakDB)
		result.Measured++
	}
	if result.Measured == 0 || energy <= 0 {
		return result, errors.New("تعذر قياس ألبوم قابل للتشغيل")
	}
	average := 10 * math.Log10(energy/seconds)
	result.LoudnessInfo = LoudnessInfo{
		IntegratedLUFS: average, TruePeakDB: peak, TargetLUFS: -18,
		GainDB: max(-24, min(12, -18-average, -1-peak)),
	}
	if result.Measured == result.Tracks {
		data, _ := json.Marshal(result)
		_ = writeChanged(path, data)
	}
	return result, nil
}
