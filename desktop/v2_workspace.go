package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Workspace state is separate from the original library and contains no provider
// credentials. Optimistic revisions prevent delayed UI writes losing newer work.
type ListeningRecord struct {
	Plays      int     `json:"plays"`
	Seconds    float64 `json:"seconds"`
	LastPlayed int64   `json:"lastPlayed"`
}
type PlaybackSession struct {
	TaskID    string   `json:"taskId"`
	Position  float64  `json:"position"`
	Queue     []string `json:"queue"`
	UpdatedAt int64    `json:"updatedAt"`
}
type DownloadPolicy struct {
	Parallel       int   `json:"parallel"`
	Scheduled      bool  `json:"scheduled"`
	StartHour      int   `json:"startHour"`
	EndHour        int   `json:"endHour"`
	WiFiOnly       bool  `json:"wifiOnly"`
	BytesPerSecond int64 `json:"bytesPerSecond"`
}
type Workspace struct {
	UIUpdatedAt int64                      `json:"uiUpdatedAt"`
	Schema      int                        `json:"schema"`
	Revision    int64                      `json:"revision"`
	UI          map[string]json.RawMessage `json:"ui"`
	Playback    PlaybackSession            `json:"playback"`
	Listening   map[string]ListeningRecord `json:"listening"`
	Downloads   DownloadPolicy             `json:"downloads"`
	UpdatedAt   int64                      `json:"updatedAt"`
}

func emptyWorkspace() Workspace {
	return Workspace{Schema: 2, UI: map[string]json.RawMessage{}, Listening: map[string]ListeningRecord{}, Downloads: DownloadPolicy{StartHour: 0, EndHour: 7}}
}
func (s *Studio) workspacePath() string { return filepath.Join(s.root, "data", "workspace-v2.json") }
func (s *Studio) readWorkspace() (Workspace, error) {
	w := emptyWorkspace()
	b, e := os.ReadFile(s.workspacePath())
	if os.IsNotExist(e) {
		return w, nil
	}
	if e != nil {
		return w, e
	}
	if len(b) > 4<<20 {
		return w, errors.New("ملف الإعدادات أكبر من الحد المسموح")
	}
	if e = json.Unmarshal(b, &w); e != nil {
		return emptyWorkspace(), e
	}
	if w.Schema != 2 {
		return w, errors.New("نسخة إعدادات غير مدعومة")
	}
	if w.UI == nil {
		w.UI = map[string]json.RawMessage{}
	}
	if w.Listening == nil {
		w.Listening = map[string]ListeningRecord{}
	}
	return w, nil
}
func (s *Studio) writeWorkspace(w Workspace) error {
	w.Schema = 2
	w.UpdatedAt = time.Now().UnixMilli()
	b, e := json.Marshal(w)
	if e != nil {
		return e
	}
	if len(b) > 4<<20 {
		return errors.New("الإعدادات أكبر من الحد المسموح")
	}
	return writeChanged(s.workspacePath(), b)
}
func (s *Studio) GetWorkspace() (Workspace, error) {
	s.v2Mu.Lock()
	defer s.v2Mu.Unlock()
	return s.readWorkspace()
}

// UI keys are intentionally enumerated: backups and device sync cannot carry
// access tokens, arbitrary script, service URLs or native paths through this API.
var workspaceUIKeys = strings.Fields("theme shape motion opacity customAccent colorSets lyricFont translationFont translationEnabled translationAuto translationTarget translationSource lyricMotion motionDuration lyricSource lyricOffsets lyricStyle lyricOffset volume speed preservePitch reverb width preamp eq bypass shuffle repeat favorites playlists libraryMode language nawaLogo performanceMode lyricFPS lyricReveal lyricScale translationScale fullscreenBackground fsBackground fsStyle fsLayout homeSections themePresets fullscreenPresets soundProfiles activeSoundProfile pitch crossfade normalizeVolume normalizationTarget showOriginal showTranslation showRomanization sleepEndOfTrack uiDensity savedDesigns artworkSelections fsVariants fsMedium fsProgress fsCardSize fsTextSize fsRotation fsBlur fsDim fsAmbient gameThemeLyrics gameThemeArt gameThemeAvatar gameThemeChoices gameThemePrevious themeEntrance lyricFps verciText verciAccent verciSecondary verciLens watchFolders lyricsScene fontSizeOriginal fontSizeTranslation playlistDesigns activePlaylist presentationBase")

func cleanWorkspaceUI(input map[string]json.RawMessage) (map[string]json.RawMessage, error) {
	out := map[string]json.RawMessage{}
	total := 0
	for _, k := range workspaceUIKeys {
		if v, ok := input[k]; ok {
			if e := validateWorkspaceValue(k, v); e != nil {
				return nil, e
			}
			if !json.Valid(v) || len(v) > 512<<10 {
				return nil, fmt.Errorf("إعداد غير صالح: %s", k)
			}
			total += len(v)
			out[k] = append(json.RawMessage(nil), v...)
		}
	}
	if total > 1<<20 {
		return nil, errors.New("إعدادات الواجهة أكبر من 1 MB")
	}
	return out, nil
}
func validDownloadPolicy(p DownloadPolicy) bool {
	return p.Parallel >= 0 && p.Parallel <= 2 && p.StartHour >= 0 && p.StartHour < 24 && p.EndHour >= 0 && p.EndHour < 24 && p.BytesPerSecond >= 0 && p.BytesPerSecond <= 1<<30
}
func (s *Studio) SaveWorkspace(revision int64, input map[string]json.RawMessage, policy DownloadPolicy) (Workspace, error) {
	clean, e := cleanWorkspaceUI(input)
	if e != nil {
		return Workspace{}, e
	}
	if !validDownloadPolicy(policy) {
		return Workspace{}, errors.New("جدولة أو حد سرعة غير صالح")
	}
	s.v2Mu.Lock()
	defer func() { s.v2Mu.Unlock(); s.enforceDownloadPolicy() }()
	w, e := s.readWorkspace()
	if e != nil {
		return w, e
	}
	if revision != w.Revision {
		return w, errors.New("تغيّرت الإعدادات؛ أعد تحميلها قبل الحفظ")
	}
	w.UI = clean
	w.UIUpdatedAt = time.Now().UnixMilli()
	w.Downloads = policy
	w.Revision++
	e = s.writeWorkspace(w)
	return w, e
}
func (s *Studio) SavePlaybackSession(p PlaybackSession) error {
	if math.IsNaN(p.Position) || math.IsInf(p.Position, 0) || p.Position < 0 || len(p.Queue) > 10000 {
		return errors.New("جلسة تشغيل غير صالحة")
	}
	if p.TaskID != "" {
		t, ok := s.task(p.TaskID)
		if !ok || t.Status != "done" {
			return errors.New("المسار غير محفوظ")
		}
		if t.Song.Duration > 0 {
			p.Position = math.Min(p.Position, t.Song.Duration)
		}
	}
	seen := map[string]bool{}
	queue := []string{}
	for _, id := range p.Queue {
		if !seen[id] {
			if t, ok := s.task(id); ok && t.Status == "done" {
				queue = append(queue, id)
				seen[id] = true
			}
		}
	}
	p.Queue = queue
	p.UpdatedAt = time.Now().UnixMilli()
	s.v2Mu.Lock()
	defer s.v2Mu.Unlock()
	w, e := s.readWorkspace()
	if e != nil {
		return e
	}
	w.Playback = p
	return s.writeWorkspace(w)
}
func (s *Studio) RecordListening(id string, seconds float64, newPlay bool) error {
	if _, ok := s.task(id); !ok {
		return nil
	}
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 || seconds > 120 {
		return errors.New("مدة الاستماع غير صالحة")
	}
	s.v2Mu.Lock()
	defer s.v2Mu.Unlock()
	w, e := s.readWorkspace()
	if e != nil {
		return e
	}
	r := w.Listening[id]
	r.Seconds += seconds
	if newPlay {
		r.Plays++
	}
	r.LastPlayed = time.Now().UnixMilli()
	w.Listening[id] = r
	return s.writeWorkspace(w)
}

type MetadataEdit struct {
	ID     string   `json:"id"`
	Title  string   `json:"title"`
	Artist string   `json:"artist"`
	Album  string   `json:"album"`
	Genres []string `json:"genres"`
}

func (s *Studio) EditLibraryMetadata(edits []MetadataEdit) error {
	if len(edits) == 0 || len(edits) > 500 {
		return errors.New("اختر بين مسار واحد و500 مسار")
	}
	for _, v := range edits {
		if strings.TrimSpace(v.Title) == "" || len(v.Title) > 1000 || len(v.Artist) > 1000 || len(v.Album) > 1000 || len(v.Genres) > 30 {
			return errors.New("معلومات مسار غير صالحة")
		}
		if _, ok := s.task(v.ID); !ok {
			return errors.New("المسار غير موجود")
		}
	}
	s.v2Mu.Lock()
	defer s.v2Mu.Unlock()
	// Keep the old identity's lyrics reachable when display tags are corrected.
	s.translationMu.Lock()
	defer s.translationMu.Unlock()
	updates := map[string]Song{}
	for _, v := range edits {
		t, _ := s.task(v.ID)
		c, e := s.readLyricCache(t.Song)
		oldKey := lyricKey(t.Song)
		song := t.Song
		song.Title = strings.TrimSpace(v.Title)
		song.Artist = strings.TrimSpace(v.Artist)
		song.Album = strings.TrimSpace(v.Album)
		song.Genres = append([]string{}, v.Genres...)
		if e == nil && oldKey != lyricKey(song) {
			for _, folder := range []string{filepath.Join("lyrics", "translations"), filepath.Join("lyrics-history", oldKey)} {
				dir := filepath.Join(s.root, "data", folder)
				entries, _ := os.ReadDir(dir)
				for _, entry := range entries {
					if entry.IsDir() {
						continue
					}
					name := entry.Name()
					if folder == filepath.Join("lyrics", "translations") && !strings.HasPrefix(name, oldKey+"-") {
						continue
					}
					destDir := dir
					if folder == filepath.Join("lyrics-history", oldKey) {
						destDir = filepath.Join(s.root, "data", "lyrics-history", lyricKey(song))
					}
					destName := strings.Replace(name, oldKey, lyricKey(song), 1)
					os.MkdirAll(destDir, 0700)
					if e = copyAtomic(filepath.Join(dir, name), filepath.Join(destDir, destName)); e != nil {
						return e
					}
				}
			}
			c.Song = song
			if e = s.writeLyricCache(c); e != nil {
				return e
			}
		}
		updates[v.ID] = song
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := append([]Task{}, s.state.Jobs...)
	for i := range s.state.Jobs {
		if song, ok := updates[s.state.Jobs[i].ID]; ok {
			s.state.Jobs[i].Song = song
			s.state.Jobs[i].GenreChecked = true
		}
	}
	s.save()
	if s.state.StorageError != "" {
		s.state.Jobs = previous
		return errors.New(s.state.StorageError)
	}
	return nil
}

type PortableTrack struct {
	Key          string            `json:"key"`
	Song         Song              `json:"song"`
	Lyrics       *LyricCache       `json:"lyrics,omitempty"`
	Translations []TranslationView `json:"translations,omitempty"`
}
type WorkspaceBackup struct {
	Schema    int               `json:"schema"`
	Version   string            `json:"version"`
	CreatedAt string            `json:"createdAt"`
	Workspace Workspace         `json:"workspace"`
	Tracks    []PortableTrack   `json:"tracks"`
	TaskKeys  map[string]string `json:"taskKeys"`
}

func portableSong(song Song) Song {
	song.URL = ""
	song.AudioURL = ""
	song.OriginalURL = ""
	song.Ref = ""
	song.Artwork = ""
	return song
}
func (s *Studio) ExportWorkspaceBackup() (WorkspaceBackup, error) {
	w, e := s.GetWorkspace()
	if e != nil {
		return WorkspaceBackup{}, e
	}
	b := WorkspaceBackup{Schema: 2, Version: "2.0.0", CreatedAt: time.Now().UTC().Format(time.RFC3339), Workspace: w, TaskKeys: map[string]string{}}
	for _, t := range s.GetState().Jobs {
		if t.Status != "done" || t.UpgradeFor != "" {
			continue
		}
		p := PortableTrack{Key: lyricKey(t.Song), Song: portableSong(t.Song)}
		b.TaskKeys[t.ID] = p.Key
		if c, e := s.readLyricCache(t.Song); e == nil {
			c.Song = p.Song
			p.Lyrics = &c
		}
		// Translation files are addressed by text hash, not by a user-supplied path.
		if rows, e := s.translationOriginal(t); e == nil {
			files, _ := filepath.Glob(filepath.Join(s.root, "data", "lyrics", "translations", lyricKey(t.Song)+"-"+translationHash(rows)+"-*.json"))
			for _, file := range files {
				raw, e := os.ReadFile(file)
				if e != nil || len(raw) > 4<<20 {
					continue
				}
				var saved TranslationView
				if json.Unmarshal(raw, &saved) != nil || !validTranslationLanguage(saved.Target, saved.RequestedSource) {
					continue
				}
				if v, e := s.readTranslation(t, rows, saved.Target, saved.RequestedSource); e == nil && v.Exists {
					p.Translations = append(p.Translations, v)
				}
			}
		}
		b.Tracks = append(b.Tracks, p)
	}
	return b, nil
}

type RestoreSummary struct {
	Matched      int `json:"matched"`
	Missing      int `json:"missing"`
	Lyrics       int `json:"lyrics"`
	Translations int `json:"translations"`
}

func (s *Studio) RestoreWorkspaceBackup(backup WorkspaceBackup, replaceEdits bool) (RestoreSummary, error) {
	var out RestoreSummary
	if backup.Schema != 2 || len(backup.Tracks) > 10000 {
		return out, errors.New("نسخة احتياطية غير مدعومة أو كبيرة جدًا")
	}
	clean, e := cleanWorkspaceUI(backup.Workspace.UI)
	if e != nil {
		return out, e
	}
	if !validDownloadPolicy(backup.Workspace.Downloads) {
		return out, errors.New("إعدادات تنزيل غير صالحة")
	}
	lookup := map[string]Task{}
	localTracks := s.GetState().Jobs
	byName := map[string][]Task{}
	for _, t := range localTracks {
		if t.Status == "done" {
			lookup[lyricKey(t.Song)] = t
			name := norm(t.Song.Title) + "|" + norm(t.Song.Artist)
			byName[name] = append(byName[name], t)
		}
	}
	// Imported audio durations can differ slightly from a catalog duration. Use
	// a unique verified recording match, never an ambiguous title-only fallback.
	for _, p := range backup.Tracks {
		if _, ok := lookup[p.Key]; ok {
			continue
		}
		matches := []Task{}
		for _, t := range byName[norm(p.Song.Title)+"|"+norm(p.Song.Artist)] {
			if t.Status == "done" && t.UpgradeFor == "" && sameSong(p.Song, t.Song) {
				matches = append(matches, t)
			}
		}
		if len(matches) == 1 {
			lookup[p.Key] = matches[0]
		}
	}
	// Validate the complete input before any change. Incoming file paths are never used.
	for _, p := range backup.Tracks {
		if p.Key != lyricKey(p.Song) {
			return out, errors.New("هوية مسار غير صالحة")
		}
		if c := p.Lyrics; c != nil {
			if len(c.Record.Plain)+len(c.Record.Synced) > 2<<20 {
				return out, errors.New("كلمات أكبر من الحد")
			}
			if len(c.Words) > 0 {
				if e := validateEditedLines(c.Words, p.Song.Duration); e != nil {
					return out, e
				}
			}
		}
		if len(p.Translations) > 400 {
			return out, errors.New("عدد ترجمات مسار أكبر من الحد")
		}
		for _, v := range p.Translations {
			if !validTranslationLanguage(v.Target, v.RequestedSource) || len(v.OriginalHash) != 64 || strings.Trim(v.OriginalHash, "0123456789abcdef") != "" || len(v.Lines) > 20000 {
				return out, errors.New("بيانات ترجمة غير صالحة في النسخة الاحتياطية")
			}
		}
	}
	// A recoverable snapshot is made before an import changes existing edits.
	if _, e = os.Stat(s.workspacePath()); e == nil {
		if e = copyAtomic(s.workspacePath(), s.workspacePath()+".before-restore"); e != nil {
			return out, e
		}
	}
	for _, p := range backup.Tracks {
		t, ok := lookup[p.Key]
		if !ok {
			out.Missing++
			continue
		}
		out.Matched++
		if p.Lyrics != nil {
			old, existing := s.readLyricCache(t.Song)
			if existing != nil || replaceEdits {
				if existing == nil {
					raw, _ := json.Marshal(old)
					if e = writeChanged(filepath.Join(s.root, "data", "lyrics-history", p.Key+"-before-restore.json"), raw); e != nil {
						return out, e
					}
				}
				c := *p.Lyrics
				c.Song = t.Song
				if e = s.writeLyricCache(c); e != nil {
					return out, e
				}
				out.Lyrics++
			}
		}
		for _, v := range p.Translations {
			if !translationLanguages[v.Target] || len(v.Lines) > 20000 {
				continue
			}
			rows, e := s.translationOriginal(t)
			if e != nil || len(rows) != len(v.Lines) || v.OriginalHash != translationHash(rows) {
				continue
			}
			matches := true
			for i := range rows {
				if rows[i].Original != v.Lines[i].Original {
					matches = false
					break
				}
			}
			if !matches {
				continue
			}
			if old, e := s.readTranslation(t, rows, v.Target, v.RequestedSource); e == nil && old.Exists && !replaceEdits {
				continue
			}
			if e = s.saveTranslation(t, v); e != nil {
				return out, e
			}
			out.Translations++
		}
	}
	remap := map[string]string{}
	for old, k := range backup.TaskKeys {
		if t, ok := lookup[k]; ok {
			remap[old] = t.ID
		}
	}
	remapUITrackIDs(clean, remap)
	s.v2Mu.Lock()
	defer s.v2Mu.Unlock()
	w, e := s.readWorkspace()
	if e != nil {
		return out, e
	}
	w.UI = clean
	w.UIUpdatedAt = time.Now().UnixMilli()
	w.Downloads = backup.Workspace.Downloads
	w.Revision++
	w.Playback = backup.Workspace.Playback
	w.Playback.TaskID = remap[w.Playback.TaskID]
	q := []string{}
	for _, old := range w.Playback.Queue {
		if id := remap[old]; id != "" {
			q = append(q, id)
		}
	}
	w.Playback.Queue = q
	for old, r := range backup.Workspace.Listening {
		if id := remap[old]; id != "" {
			existing := w.Listening[id]
			if r.LastPlayed > existing.LastPlayed {
				w.Listening[id] = r
			}
		}
	}
	return out, s.writeWorkspace(w)
}
func remapUITrackIDs(ui map[string]json.RawMessage, ids map[string]string) {
	var fav []string
	if json.Unmarshal(ui["favorites"], &fav) == nil {
		out := []string{}
		for _, old := range fav {
			if id := ids[old]; id != "" {
				out = append(out, id)
			}
		}
		ui["favorites"], _ = json.Marshal(out)
	}
	var lists []struct {
		ID     string   `json:"id"`
		Name   string   `json:"name"`
		Tracks []string `json:"tracks"`
	}
	if json.Unmarshal(ui["playlists"], &lists) == nil {
		for i := range lists {
			out := []string{}
			for _, old := range lists[i].Tracks {
				if id := ids[old]; id != "" {
					out = append(out, id)
				}
			}
			lists[i].Tracks = out
		}
		ui["playlists"], _ = json.Marshal(lists)
	}
	for _, key := range []string{"lyricOffsets", "savedDesigns"} {
		var values map[string]json.RawMessage
		if json.Unmarshal(ui[key], &values) == nil {
			out := map[string]json.RawMessage{}
			for old, v := range values {
				if id := ids[old]; id != "" {
					out[id] = v
				}
			}
			ui[key], _ = json.Marshal(out)
		}
	}
}

func downloadAllowedAt(p DownloadPolicy, at time.Time) bool {
	if !p.Scheduled || p.StartHour == p.EndHour {
		return true
	}
	h := at.Hour()
	if p.StartHour < p.EndHour {
		return h >= p.StartHour && h < p.EndHour
	}
	return h >= p.StartHour || h < p.EndHour
}
func (s *Studio) downloadPolicy() DownloadPolicy {
	w, e := s.GetWorkspace()
	if e != nil {
		return DownloadPolicy{}
	}
	return w.Downloads
}

// Stable sorting is also used by the desktop and Android search presentation.
func groupSearchSongs(songs []Song) [][]Song {
	groups := [][]Song{}
	for _, song := range songs {
		matched := -1
		for i, g := range groups {
			if sameRecordingVersion(g[0], song) {
				matched = i
				break
			}
		}
		if matched < 0 {
			groups = append(groups, []Song{song})
		} else {
			groups[matched] = append(groups[matched], song)
		}
	}
	for _, g := range groups {
		sort.SliceStable(g, func(i, j int) bool { return g[i].QualityVerified && !g[j].QualityVerified })
	}
	return groups
}

var versionWords = []string{"live", "remix", "remaster", "acoustic", "instrumental", "karaoke", "radio edit", "sped up", "slowed", "clean", "explicit", "حفلة", "مباشر"}

func recordingVersion(title string) string {
	title = strings.ToLower(title)
	tags := []string{}
	for _, v := range versionWords {
		if strings.Contains(title, v) {
			tags = append(tags, v)
		}
	}
	return strings.Join(tags, "|")
}
func sameRecordingVersion(a, b Song) bool {
	if recordingVersion(a.Title) != recordingVersion(b.Title) {
		return false
	}
	if a.Duration > 0 && b.Duration > 0 && math.Abs(a.Duration-b.Duration) > math.Max(3, a.Duration*.02) {
		return false
	}
	if a.ISRC != "" && b.ISRC != "" {
		return strings.EqualFold(a.ISRC, b.ISRC)
	}
	return norm(a.Title) == norm(b.Title) && norm(a.Artist) == norm(b.Artist)
}
