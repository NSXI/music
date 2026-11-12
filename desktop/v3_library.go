package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math"
	"math/bits"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type LibraryFinding struct {
	ID      string  `json:"id"`
	Song    Song    `json:"song"`
	Quality Quality `json:"quality"`
	Missing bool    `json:"missing"`
	Cover   bool    `json:"cover"`
	Timing  string  `json:"timing"`
}

type RecordingGroup struct {
	Tracks []LibraryFinding `json:"tracks"`
	Reason string           `json:"reason"`
}

type LibraryReview struct {
	Tracks   []LibraryFinding `json:"tracks"`
	Groups   []RecordingGroup `json:"groups"`
	Relinked int              `json:"relinked"`
}

func (s *Studio) ReviewLibrary(relink bool) (LibraryReview, error) {
	out := LibraryReview{Tracks: []LibraryFinding{}, Groups: []RecordingGroup{}}
	jobs := s.GetState().Jobs
	if relink {
		n, err := s.relinkLibraryFiles(jobs)
		if err != nil {
			return out, err
		}
		out.Relinked = n
		jobs = s.GetState().Jobs
	}
	byIdentity := map[string][]Task{}
	byHash := map[string][]LibraryFinding{}
	findings := map[string]LibraryFinding{}
	for _, task := range jobs {
		if task.Status != "done" || task.UpgradeFor != "" {
			continue
		}
		_, audioError := os.Stat(task.Path)
		_, coverError := os.Stat(task.Cover)
		finding := LibraryFinding{ID: task.ID, Song: portableSong(task.Song), Quality: task.Quality, Missing: audioError != nil, Cover: coverError == nil, Timing: "missing"}
		if cache, err := s.readLyricCache(task.Song); err == nil {
			switch {
			case len(cache.Words) > 0:
				finding.Timing = "word"
			case cache.Record.Synced != "":
				finding.Timing = "line"
			case cache.Record.Plain != "":
				finding.Timing = "plain"
			}
		}
		out.Tracks = append(out.Tracks, finding)
		findings[task.ID] = finding
		if task.Hash != "" {
			byHash[task.Hash] = append(byHash[task.Hash], finding)
		}
		identity := norm(task.Song.Title) + "|" + norm(task.Song.Artist) + "|" + recordingVersion(task.Song.Title)
		if task.Song.ISRC != "" {
			identity = strings.ToUpper(task.Song.ISRC) + "|" + recordingVersion(task.Song.Title)
		}
		if norm(task.Song.Title) != "" && norm(task.Song.Artist) != "" {
			byIdentity[identity] = append(byIdentity[identity], task)
		}
	}
	for _, group := range byHash {
		if len(group) > 1 {
			out.Groups = append(out.Groups, RecordingGroup{Tracks: group, Reason: "file"})
		}
	}
	for _, candidates := range byIdentity {
		for len(candidates) > 1 {
			first := candidates[0]
			group := []LibraryFinding{findings[first.ID]}
			remaining := []Task{}
			differentFile := false
			for _, task := range candidates[1:] {
				if sameRecordingVersion(first.Song, task.Song) {
					group = append(group, findings[task.ID])
					differentFile = differentFile || task.Hash != first.Hash || task.Hash == ""
				} else {
					remaining = append(remaining, task)
				}
			}
			if len(group) > 1 && differentFile {
				sort.SliceStable(group, func(i, j int) bool {
					a, b := group[i].Quality, group[j].Quality
					if a.Lossless != b.Lossless {
						return a.Lossless
					}
					if a.Lossless && a.Bits != b.Bits {
						return a.Bits > b.Bits
					}
					if a.Lossless && a.Rate != b.Rate {
						return a.Rate > b.Rate
					}
					return a.Bitrate > b.Bitrate
				})
				out.Groups = append(out.Groups, RecordingGroup{Tracks: group, Reason: "recording"})
			}
			candidates = remaining
		}
	}
	sort.SliceStable(out.Groups, func(i, j int) bool { return out.Groups[i].Tracks[0].Song.Title < out.Groups[j].Tracks[0].Song.Title })
	return out, nil
}

func (s *Studio) relinkLibraryFiles(jobs []Task) (int, error) {
	missing := map[int64][]Task{}
	for _, task := range jobs {
		if task.Status == "done" && task.Hash != "" && task.Bytes > 0 {
			if _, err := os.Stat(task.Path); os.IsNotExist(err) {
				missing[task.Bytes] = append(missing[task.Bytes], task)
			}
		}
	}
	if len(missing) == 0 {
		return 0, nil
	}
	prefs := s.GetState().Preferences
	roots := append(append([]string{}, prefs.LibraryFolders...), prefs.Folder)
	matches := map[string]string{}
	seen := map[string]bool{}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	for _, root := range roots {
		if root == "" {
			continue
		}
		if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if walkErr != nil || entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || seen[path] {
				return nil
			}
			seen[path] = true
			info, err := entry.Info()
			if err != nil || !info.Mode().IsRegular() || len(missing[info.Size()]) == 0 {
				return nil
			}
			hash := fileHash(path)
			for _, old := range missing[info.Size()] {
				if old.Hash == hash && matches[old.ID] == "" {
					matches[old.ID] = path
				}
			}
			return nil
		}); err != nil {
			return 0, err
		}
	}
	if len(matches) == 0 {
		return 0, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	relinked := 0
	for i := range s.state.Jobs {
		task := &s.state.Jobs[i]
		if path := matches[task.ID]; path != "" {
			if _, err := os.Stat(task.Path); os.IsNotExist(err) {
				task.Path = path
				relinked++
			}
		}
	}
	if relinked > 0 {
		s.save()
	}
	return relinked, nil
}

type AudioFingerprintMatch struct {
	First      string  `json:"first"`
	Second     string  `json:"second"`
	Similarity float64 `json:"similarity"`
}

// Match a bounded selection. These are candidates for review, never merges.
func (s *Studio) CompareAudioFingerprints(ids []string) ([]AudioFingerprintMatch, error) {
	if len(ids) < 2 || len(ids) > 20 {
		return nil, errors.New("اختر من مسارين إلى 20 مسارًا")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	release, err := s.acquireHeavyTask(ctx, "audio-fingerprint")
	if err != nil {
		return nil, err
	}
	defer release()
	type item struct {
		task Task
		data []uint32
	}
	items := []item{}
	seen := map[string]bool{}
	for _, id := range ids {
		task, ok := s.task(id)
		if !ok || task.Status != "done" || seen[id] {
			continue
		}
		seen[id] = true
		fp, err := s.audioFingerprint(ctx, task)
		if err != nil {
			return nil, err
		}
		items = append(items, item{task, fp})
	}
	matches := []AudioFingerprintMatch{}
	for i, a := range items {
		for _, b := range items[i+1:] {
			if recordingVersion(a.task.Song.Title) != recordingVersion(b.task.Song.Title) ||
				math.Abs(a.task.Song.Duration-b.task.Song.Duration) > 4 {
				continue
			}
			if score := fingerprintSimilarity(a.data, b.data); score >= .92 {
				matches = append(matches, AudioFingerprintMatch{a.task.ID, b.task.ID, score})
			}
		}
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].Similarity > matches[j].Similarity })
	return matches, nil
}

func (s *Studio) audioFingerprint(ctx context.Context, task Task) ([]uint32, error) {
	info, err := os.Stat(task.Path)
	if err != nil {
		return nil, err
	}
	cache := filepath.Join(s.root, "data", "fingerprints", key(task.Path+"|"+info.ModTime().String()+"|"+task.Hash)+".json")
	var features []uint32
	if raw, err := os.ReadFile(cache); err == nil && len(raw) < 64<<10 && json.Unmarshal(raw, &features) == nil {
		return features, nil
	}
	raw, err := s.command(ctx, "ffmpeg", "-v", "error", "-nostdin", "-i", task.Path, "-t", "45", "-map", "0:a:0", "-ac", "1", "-ar", "8000", "-f", "s16le", "-").Output()
	if err != nil {
		return nil, errors.New("تعذر قراءة الصوت للبصمة: " + task.Song.Title)
	}
	features = spectralFingerprint(raw)
	if data, err := json.Marshal(features); err == nil {
		_ = writeChanged(cache, data)
	}
	return features, nil
}

func spectralFingerprint(raw []byte) []uint32 {
	const frame = 1024
	frequencies := [...]float64{80, 108, 146, 197, 266, 359, 485, 655, 884, 1193, 1610, 2174, 2935}
	var previous [len(frequencies)]float64
	features := []uint32{}
	for at := 0; at+frame*2 <= len(raw); at += frame * 2 {
		var window [frame]float64
		energy := 0.0
		for i := range window {
			sample := float64(int16(binary.LittleEndian.Uint16(raw[at+2*i:]))) / 32768
			energy += sample * sample
			window[i] = sample * (.5 - .5*math.Cos(2*math.Pi*float64(i)/float64(frame-1)))
		}
		if energy < .02 && len(features) == 0 {
			continue
		}
		var powers [len(frequencies)]float64
		for i, freq := range frequencies {
			coef := 2 * math.Cos(2*math.Pi*freq/8000)
			a, b := 0.0, 0.0
			for _, sample := range window {
				a, b = sample+coef*a-b, a
			}
			powers[i] = math.Log1p(math.Max(0, a*a+b*b-coef*a*b))
		}
		var feature uint32
		for i := 0; i < len(powers)-1; i++ {
			diff := powers[i] - powers[i+1]
			if diff > 0 {
				feature |= 1 << i
			}
			if diff > previous[i]-previous[i+1] {
				feature |= 1 << (i + 12)
			}
		}
		features = append(features, feature)
		previous = powers
	}
	return features
}

func fingerprintSimilarity(a, b []uint32) float64 {
	best := 0.0
	for shift := -8; shift <= 8; shift++ {
		offA, offB := max(0, -shift), max(0, shift)
		count := min(len(a)-offA, len(b)-offB)
		if count < 28 {
			continue
		}
		distance := 0
		distinct := map[uint32]bool{}
		for i := 2; i < count; i++ {
			x, y := a[i+offA], b[i+offB]
			distance += bits.OnesCount32(x ^ y)
			distinct[x] = true
		}
		if len(distinct) >= 8 {
			best = max(best, 1-float64(distance)/float64(24*(count-2)))
		}
	}
	return best
}
