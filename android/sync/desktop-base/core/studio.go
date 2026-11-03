package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	wr "github.com/wailsapp/wails/v2/pkg/runtime"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
)

type Song struct {
	Genres          []string `json:"genres,omitempty"`
	OriginalURL     string   `json:"originalUrl,omitempty"`
	Size            int64    `json:"size,omitempty"`
	Codec           string   `json:"codec,omitempty"`
	Bitrate         int      `json:"bitrate,omitempty"`
	SampleRate      int      `json:"sampleRate,omitempty"`
	Bits            int      `json:"bits,omitempty"`
	SizeEstimated   bool     `json:"sizeEstimated,omitempty"`
	QualityVerified bool     `json:"qualityVerified,omitempty"`
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	Artist          string   `json:"artist"`
	Album           string   `json:"album"`
	Duration        float64  `json:"duration"`
	Artwork         string   `json:"artwork"`
	Provider        string   `json:"provider"`
	URL             string   `json:"url"`
	AudioURL        string   `json:"audioUrl,omitempty"`
	Ref             string   `json:"ref,omitempty"`
	ISRC            string   `json:"isrc,omitempty"`
}
type Quality struct {
	Codec    string  `json:"codec"`
	Bits     int     `json:"bits"`
	Rate     int     `json:"rate"`
	Bitrate  int     `json:"bitrate"`
	Channels int     `json:"channels"`
	Lossless bool    `json:"lossless"`
	Duration float64 `json:"duration"`
}
type Task struct {
	MetadataPending bool     `json:"metadataPending,omitempty"`
	LocalModified   int64    `json:"localModified,omitempty"`
	GenreChecked    bool     `json:"genreChecked,omitempty"`
	LyricsSource    string   `json:"lyricsSource,omitempty"`
	ID              string   `json:"id"`
	Song            Song     `json:"song"`
	Status          string   `json:"status"`
	Phase           string   `json:"phase"`
	Error           string   `json:"error"`
	Bytes           int64    `json:"bytes"`
	Total           int64    `json:"total"`
	RetryAt         int64    `json:"retryAt"`
	ETag            string   `json:"etag,omitempty"`
	TransferURL     string   `json:"transferUrl,omitempty"`
	Path            string   `json:"path"`
	Cover           string   `json:"cover"`
	Lyrics          string   `json:"lyrics"`
	Warning         string   `json:"warning"`
	Hash            string   `json:"hash"`
	Source          string   `json:"source"`
	Log             []string `json:"log"`
	Quality         Quality  `json:"quality"`
	Local           bool     `json:"local"`
}
type Preferences struct {
	ResourceMode           string   `json:"resourceMode,omitempty"`
	LrcAPIURL              string   `json:"lrcapiURL,omitempty"`
	MetadataSourcesVersion int      `json:"metadataSourcesVersion,omitempty"`
	LibraryFolders         []string `json:"libraryFolders,omitempty"`
	AutoTranslate          bool     `json:"autoTranslate"`
	TranslationTarget      string   `json:"translationTarget"`
	TranslationSource      string   `json:"translationSource"`
	LyricProviders         []string `json:"lyricProviders"`
	AutoAlign              bool     `json:"autoAlign"`
	AlignmentLanguage      string   `json:"alignmentLanguage"`
	Folder                 string   `json:"folder"`
	Lyrics                 bool     `json:"lyrics"`
	LosslessOnly           bool     `json:"losslessOnly"`
	TidalURL               string   `json:"tidalUrl"`
	AutoMatch              bool     `json:"autoMatch"`
	SpotifyClientID        string   `json:"spotifyClientId"`
}
type Snapshot struct {
	Resource             ResourceState        `json:"resource"`
	MetadataConfigured   map[string]bool      `json:"metadataConfigured,omitempty"`
	FolderImport         FolderImportState    `json:"folderImport"`
	TranslationQueue     []TranslationRequest `json:"translationQueue,omitempty"`
	Translation          TranslationState     `json:"translation"`
	AlignmentQueue       []string             `json:"alignmentQueue,omitempty"`
	Cooldowns            map[string]int64     `json:"cooldowns,omitempty"`
	MusixmatchConfigured bool                 `json:"musixmatchConfigured"`
	Root                 string               `json:"root,omitempty"`
	Alignment            AlignmentState       `json:"alignment"`
	Jobs                 []Task               `json:"jobs"`
	Preferences          Preferences          `json:"preferences"`
	Paused               bool                 `json:"paused"`
	StorageError         string               `json:"storageError"`
	SpotifyConnected     bool                 `json:"spotifyConnected"`
}
type Result struct {
	ProviderCounts map[string]int    `json:"providerCounts,omitempty"`
	ProviderErrors map[string]string `json:"providerErrors,omitempty"`
	Title          string            `json:"title"`
	Tracks         []Song            `json:"tracks"`
	Notice         string            `json:"notice"`
	Complete       bool              `json:"complete"`
	Total          int               `json:"total"`
}
type Studio struct {
	searchMu               sync.Mutex
	searchCancel           context.CancelFunc
	searchGeneration       uint64
	metadataWake           chan struct{}
	analysisSlot           chan struct{}
	resourceState          ResourceState
	metadataKeys           map[string]string
	importMu               sync.Mutex
	folderImport           FolderImportState
	folderCancel           context.CancelFunc
	screenMu               sync.Mutex
	fullscreenWasMaximised bool
	previewServer          *http.Server
	previewAddress         string
	previewServerError     string
	previewMu              sync.Mutex
	previews               map[string]previewSession
	previewSongs           map[string]Song
	sourceTestBases        map[string]string
	searchExtractSlots     chan struct{}
	translationMu          sync.Mutex
	translationState       TranslationState
	translationCancel      context.CancelFunc
	translationFailures    map[string]bool
	closing                bool
	musixmatchKey          string
	lyricTestBases         map[string]string
	alignMu                sync.Mutex
	alignCancel            context.CancelFunc
	alignState             AlignmentState

	mu             sync.Mutex
	root           string
	ctx            context.Context
	client         *http.Client
	state          Snapshot
	known          map[string]Song
	wake           chan struct{}
	done           chan struct{}
	halt           context.CancelFunc
	activeCancel   context.CancelFunc
	active         string
	action         string
	spotifyToken   string
	spotifyRefresh string
	spotifyExpiry  time.Time
	oauthMu        sync.Mutex
	mediaMu        sync.Mutex
}

func NewStudio(root string) *Studio {
	s := &Studio{root: root, client: &http.Client{Timeout: 90 * time.Second}, known: map[string]Song{}, wake: make(chan struct{}, 1), metadataWake: make(chan struct{}, 1), searchExtractSlots: make(chan struct{}, 2), done: make(chan struct{})}
	s.state = Snapshot{Root: root, Jobs: []Task{}, Preferences: Preferences{AutoTranslate: true, TranslationTarget: "ar", TranslationSource: "auto", Folder: filepath.Join(root, "Downloads"), Lyrics: true, AutoMatch: true, AutoAlign: false, AlignmentLanguage: "auto", LyricProviders: append([]string{}, defaultLyricProviders...)}}
	raw, err := os.ReadFile(filepath.Join(root, "data", "library.json"))
	if err == nil {
		loaded := s.state
		if json.Unmarshal(raw, &loaded) == nil {
			s.state = loaded
		} else {
			backup := filepath.Join(root, "data", fmt.Sprintf("library-corrupt-%d.json", time.Now().UnixNano()))
			if e := os.WriteFile(backup, raw, 0600); e != nil {
				s.state.StorageError = "تعذر حفظ نسخة من سجل المكتبة التالف"
			} else {
				s.state.StorageError = "تعذر قراءة سجل المكتبة السابق؛ حُفظت نسخة منه للمراجعة"
			}
		}
	} else if !os.IsNotExist(err) {
		s.state.StorageError = err.Error()
	}
	if s.state.Jobs == nil {
		s.state.Jobs = []Task{}
	}
	oldRoot := s.state.Root
	if oldRoot != "" && !strings.EqualFold(filepath.Clean(oldRoot), filepath.Clean(root)) {
		rebase := func(path string) string {
			if path == "" {
				return path
			}
			rel, e := filepath.Rel(oldRoot, path)
			if e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel) {
				return filepath.Join(root, rel)
			}
			return path
		}
		s.state.Preferences.Folder = rebase(s.state.Preferences.Folder)
		for i := range s.state.Jobs {
			j := &s.state.Jobs[i]
			j.Path = rebase(j.Path)
			j.Cover = rebase(j.Cover)
			j.Lyrics = rebase(j.Lyrics)
		}
	}
	s.state.Root = root
	if s.state.Preferences.Folder == "" {
		s.state.Preferences.Folder = filepath.Join(root, "Downloads")
	}
	if s.state.Preferences.TranslationTarget == "" {
		s.state.Preferences.TranslationTarget = "ar"
	}
	if s.state.Preferences.TranslationSource == "" {
		s.state.Preferences.TranslationSource = "auto"
	}
	if !validTranslationLanguage(s.state.Preferences.TranslationTarget, s.state.Preferences.TranslationSource) {
		s.state.Preferences.TranslationTarget = "ar"
		s.state.Preferences.TranslationSource = "auto"
	}
	s.state.Preferences.ResourceMode = normalizeResourceMode(s.state.Preferences.ResourceMode)
	if len(s.state.Preferences.LyricProviders) == 0 {
		s.state.Preferences.LyricProviders = append([]string{}, defaultLyricProviders...)
	}
	if s.state.Preferences.MetadataSourcesVersion < 1 {
		seen := map[string]bool{}
		for _, id := range s.state.Preferences.LyricProviders {
			seen[id] = true
		}
		for _, id := range additionalLyricOrder {
			if id != "audd" && !seen[id] {
				s.state.Preferences.LyricProviders = append(s.state.Preferences.LyricProviders, id)
				seen[id] = true
			}
		}
		s.state.Preferences.MetadataSourcesVersion = 1
	}
	if s.state.Preferences.MetadataSourcesVersion < 2 {
		seen := map[string]bool{}
		for _, id := range s.state.Preferences.LyricProviders {
			seen[id] = true
		}
		for _, id := range regionalLyricOrder {
			if !seen[id] {
				s.state.Preferences.LyricProviders = append(s.state.Preferences.LyricProviders, id)
				seen[id] = true
			}
		}
		s.state.Preferences.MetadataSourcesVersion = 2
	}

	if s.state.Preferences.MetadataSourcesVersion < 3 {
		order := []string{}
		seen := map[string]bool{}
		for _, id := range s.state.Preferences.LyricProviders {
			if !seen[id] {
				order = append(order, id)
				seen[id] = true
			}
		}
		for _, id := range []string{"amll", "betterlyrics"} {
			if !seen[id] {
				order = append(order, id)
				seen[id] = true
			}
		}
		s.state.Preferences.LyricProviders = order
		s.state.Preferences.MetadataSourcesVersion = 3
	}
	for i := range s.state.Jobs {
		t := &s.state.Jobs[i]
		if t.Status == "working" {
			t.Status = "queued"
		}
		t.Phase = ""
		s.known[t.Song.ID] = t.Song
	}
	// Timing analysis starts only from an explicit user action. Keep existing
	// source timing, accepted results and manual offsets; drop old auto work.
	s.state.Preferences.AutoAlign = false
	s.state.AlignmentQueue = nil
	s.loadAuth()
	return s
}

func (s *Studio) start(ctx context.Context) {
	s.ctx = ctx
	s.startPreviewServer()
	run, halt := context.WithCancel(ctx)
	s.halt = halt
	go s.loop(run)
	go s.nextAlignment()
	go s.nextTranslation()
	go s.refreshLibraryGenres(run)
	go s.metadataLoop(run)
	select {
	case s.metadataWake <- struct{}{}:
	default:
	}
	s.signal()
}
func (s *Studio) stop(_ context.Context) {
	s.previewMu.Lock()
	if s.previewServer != nil {
		s.previewServer.Close()
	}
	s.previewMu.Unlock()
	if s.halt != nil {
		s.halt()
	}
	s.mu.Lock()
	s.closing = true
	if s.folderCancel != nil {
		s.folderCancel()
	}
	if s.translationCancel != nil {
		s.translationCancel()
	}
	if s.alignCancel != nil {
		s.alignCancel()
	}
	if s.activeCancel != nil {
		s.activeCancel()
	}
	s.mu.Unlock()
	if s.halt != nil {
		select {
		case <-s.done:
		case <-time.After(4 * time.Second):
		}
	}
	s.mu.Lock()
	s.save()
	s.mu.Unlock()
}
func (s *Studio) signal() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}
func (s *Studio) save() {
	dir := filepath.Join(s.root, "data")
	if e := os.MkdirAll(dir, 0755); e != nil {
		s.state.StorageError = e.Error()
		return
	}
	s.state.Root = s.root
	b, e := json.MarshalIndent(s.state, "", "  ")
	if e == nil {
		e = os.WriteFile(filepath.Join(dir, "library.tmp"), b, 0600)
	}
	if e == nil {
		e = os.Rename(filepath.Join(dir, "library.tmp"), filepath.Join(dir, "library.json"))
	}
	if e != nil {
		s.state.StorageError = e.Error()
	} else {
		s.state.StorageError = ""
	}
}
func (s *Studio) GetState() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, _ := json.Marshal(s.state)
	var r Snapshot
	json.Unmarshal(b, &r)
	r.SpotifyConnected = s.spotifyToken != ""
	r.MusixmatchConfigured = s.musixmatchKey != ""
	r.MetadataConfigured = map[string]bool{}
	for provider, value := range s.metadataKeys {
		r.MetadataConfigured[provider] = value != ""
	}
	r.Resource = s.resourceState
	r.Resource.Mode = normalizeResourceMode(s.state.Preferences.ResourceMode)
	r.FolderImport = s.folderImport
	r.FolderImport.Errors = append([]string(nil), s.folderImport.Errors...)
	r.Alignment = s.alignState
	r.Alignment.Ready = s.alignmentReady()
	r.Alignment.Pending = len(s.state.AlignmentQueue)
	r.Translation = s.translationState
	r.Translation.Ready = s.translationReady()
	r.Translation.Pending = len(s.state.TranslationQueue)
	return r
}
func (s *Studio) edit(id string, f func(*Task)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.state.Jobs {
		if s.state.Jobs[i].ID == id {
			f(&s.state.Jobs[i])
			return
		}
	}
}
func (s *Studio) task(id string) (Task, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.state.Jobs {
		if t.ID == id {
			return t, true
		}
	}
	if song, ok := s.previewSongs[id]; ok {
		return Task{ID: id, Song: song, Status: "preview", Log: []string{}}, true
	}
	return Task{}, false
}
func (s *Studio) record(id, msg string) {
	s.edit(id, func(t *Task) {
		t.Log = append(t.Log, time.Now().Format("15:04:05")+"  "+msg)
		if len(t.Log) > 60 {
			t.Log = t.Log[len(t.Log)-60:]
		}
	})
	s.mu.Lock()
	s.save()
	s.mu.Unlock()
}
func key(v string) string { h := sha256.Sum256([]byte(v)); return hex.EncodeToString(h[:10]) }
func norm(v string) string {
	return strings.Join(strings.Fields(strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return ' '
	}, v)), " ")
}
func sameSong(a, b Song) bool {
	if a.ID == b.ID {
		return true
	}
	if norm(a.Title) == "" || norm(a.Title) != norm(b.Title) || norm(a.Artist) == "" || norm(a.Artist) != norm(b.Artist) {
		return false
	}
	if a.Duration <= 0 || b.Duration <= 0 {
		return false
	}
	d := a.Duration - b.Duration
	if d < 0 {
		d = -d
	}
	return d <= 3 && (a.ISRC == "" || b.ISRC == "" || a.ISRC == b.ISRC)
}
func safe(v string) string {
	v = strings.Trim(strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, v), " .")
	r := []rune(v)
	if len(r) > 100 {
		v = string(r[:100])
	}
	if v == "" {
		return "Audio"
	}
	return v
}
func (s *Studio) remember(r Result) Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := map[string]bool{}
	items := []Song{}
	for _, v := range r.Tracks {
		if v.ID == "" {
			v.ID = key(v.Provider + v.URL + v.Title + v.Artist)
		}
		if !seen[v.ID] {
			s.known[v.ID] = v
			seen[v.ID] = true
			items = append(items, v)
		}
	}
	r.Tracks = items
	return r
}
func (s *Studio) Enqueue(ids []string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range ids {
		if _, ok := s.known[id]; !ok {
			return 0, errors.New("أعد البحث عن المسار قبل إضافته")
		}
	}
	n := 0
	for _, id := range ids {
		v := s.known[id]
		found := false
		for _, t := range s.state.Jobs {
			if sameSong(t.Song, v) {
				found = true
				break
			}
		}
		if found {
			continue
		}
		t := Task{ID: key("task:" + v.ID), Song: v, Status: "queued", Log: []string{}, Source: v.Provider}
		s.state.Jobs = append(s.state.Jobs, t)
		n++
	}
	s.save()
	s.signal()
	return n, nil
}
func (s *Studio) Control(id, action string) error {
	if action == "remove" && id == "" {
		return errors.New("اختر مسارًا لإزالته من السجل")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.state.Jobs {
		t := &s.state.Jobs[i]
		if id != "" && id != t.ID {
			continue
		}
		switch action {
		case "pause":
			if t.Status == "queued" || t.Status == "working" {
				t.Status = "paused"
			}
		case "cancel":
			if t.Status != "done" {
				t.Status = "cancelled"
			}
		case "retry", "resume":
			if t.Status == "failed" || t.Status == "cancelled" || t.Status == "paused" {
				t.Status = "queued"
				t.Error = ""
				t.RetryAt = 0
			}
		case "remove":
			if t.ID == s.active {
				return errors.New("أوقف المسار أولًا قبل إزالته")
			}
			s.state.Jobs = append(s.state.Jobs[:i], s.state.Jobs[i+1:]...)
			s.save()
			return nil
		}
		if s.active == t.ID && (action == "pause" || action == "cancel") {
			s.action = action
			if s.activeCancel != nil {
				s.activeCancel()
			}
		}
	}
	s.save()
	s.signal()
	return nil
}
func (s *Studio) SetPreferences(p Preferences) error {
	p.AutoAlign = false
	if p.TranslationTarget == "" {
		p.TranslationTarget = "ar"
	}
	if p.TranslationSource == "" {
		p.TranslationSource = "auto"
	}
	if !validTranslationLanguage(p.TranslationTarget, p.TranslationSource) {
		return errors.New("لغة ترجمة غير مدعومة")
	}
	if p.Folder == "" {
		return errors.New("اختر مجلد تنزيل")
	}
	if p.LrcAPIURL != "" {
		if err := validWebURL(p.LrcAPIURL); err != nil {
			return err
		}
		p.LrcAPIURL = strings.TrimRight(p.LrcAPIURL, "/")
	}
	if p.TidalURL != "" {
		if err := validWebURL(p.TidalURL); err != nil {
			return err
		}
		p.TidalURL = strings.TrimRight(p.TidalURL, "/")
	}
	if err := os.MkdirAll(p.Folder, 0755); err != nil {
		return err
	}
	s.mu.Lock()
	p.LibraryFolders = append([]string(nil), s.state.Preferences.LibraryFolders...)
	p.MetadataSourcesVersion = s.state.Preferences.MetadataSourcesVersion
	p.ResourceMode = normalizeResourceMode(p.ResourceMode)
	s.state.Preferences = p
	s.save()
	s.mu.Unlock()
	return nil
}
func (s *Studio) ChooseFolder() (string, error) {
	p := s.GetState().Preferences.Folder
	os.MkdirAll(p, 0755)
	v, e := wr.OpenDirectoryDialog(s.ctx, wr.OpenDialogOptions{Title: "مجلد التنزيل", DefaultDirectory: p})
	if e == nil && v != "" {
		s.mu.Lock()
		s.state.Preferences.Folder = v
		s.save()
		s.mu.Unlock()
	}
	return v, e
}
func (s *Studio) OpenFolder() error {
	p := s.GetState().Preferences.Folder
	os.MkdirAll(p, 0755)
	return hiddenCommand(context.Background(), "explorer.exe", p).Start()
}
func (s *Studio) Window(action string) {
	if s.ctx == nil {
		return
	}
	switch action {
	case "close":
		wr.Quit(s.ctx)
	case "min":
		wr.WindowMinimise(s.ctx)
	case "max":
		wr.WindowToggleMaximise(s.ctx)
	case "fullscreen":
		_ = s.SetLyricsFullscreen(!wr.WindowIsFullscreen(s.ctx))
	}
}
func (s *Studio) OpenURL(address string) error {
	if err := validWebURL(address); err != nil {
		return err
	}
	wr.BrowserOpenURL(s.ctx, address)
	return nil
}
func (s *Studio) SpatialSettings() error {
	return hiddenCommand(context.Background(), "cmd.exe", "/c", "start", "", "ms-settings:sound").Start()
}
func (s *Studio) loop(ctx context.Context) {
	defer close(s.done)
	for {
		if ctx.Err() != nil {
			return
		}
		s.mu.Lock()
		idx := -1
		for i, t := range s.state.Jobs {
			if t.Status == "queued" {
				idx = i
				break
			}
		}
		if idx < 0 {
			s.mu.Unlock()
			select {
			case <-ctx.Done():
				return
			case <-s.wake:
			}
			continue
		}
		t := s.state.Jobs[idx]
		s.state.Jobs[idx].Status = "working"
		run, cancel := context.WithCancel(ctx)
		s.active = t.ID
		s.activeCancel = cancel
		s.action = ""
		s.save()
		s.mu.Unlock()
		err := s.execute(run, t)
		cancel()
		s.mu.Lock()
		for i := range s.state.Jobs {
			j := &s.state.Jobs[i]
			if j.ID != t.ID {
				continue
			}
			j.Phase = ""
			j.RetryAt = 0
			if err == nil {
				j.Status = "done"
				j.Error = ""
			} else if s.action == "pause" {
				j.Status = "paused"
			} else if s.action == "cancel" {
				j.Status = "cancelled"
			} else if ctx.Err() != nil {
				j.Status = "queued"
			} else {
				j.Status = "failed"
				j.Error = err.Error()
			}
		}
		s.active = ""
		s.activeCancel = nil
		s.action = ""
		s.save()
		s.mu.Unlock()
	}
}
func fileHash(path string) string {
	f, e := os.Open(path)
	if e != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}
func (s *Studio) execute(ctx context.Context, t Task) error {
	p := s.GetState().Preferences
	if t.Song.Provider == "spotify" && t.Song.Ref != "" && t.Song.Artwork == "" {
		if r, e := s.spotifyEmbed(ctx, "track", t.Song.Ref); e == nil && len(r.Tracks) > 0 {
			t.Song.Artwork = r.Tracks[0].Artwork
			s.edit(t.ID, func(j *Task) { j.Song.Artwork = t.Song.Artwork })
		}
	}
	s.edit(t.ID, func(j *Task) { j.Phase = "البحث عن أفضل مصدر مطابق" })
	s.record(t.ID, "بدء محاولة المصادر")
	candidates, err := s.resolve(ctx, t.Song, p)
	if err != nil {
		return err
	}
	if len(candidates) == 0 {
		return errors.New("لم توجد نسخة مطابقة متاحة. راجع المصادر أو أضف رابط ملف صوتي مباشر.")
	}
	var failures []string
	var bestPath string
	var bestQuality Quality
	var bestSource Song
	var losslessDeadline time.Time
	for _, c := range candidates {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if bestPath != "" && !losslessDeadline.IsZero() && time.Now().After(losslessDeadline) {
			break
		}
		s.record(t.ID, "محاولة "+c.Provider)
		s.edit(t.ID, func(j *Task) { j.Source = c.Provider; j.Phase = "تنزيل الصوت" })
		attemptCtx := ctx
		cancelAttempt := func() {}
		if bestPath != "" {
			attemptCtx, cancelAttempt = context.WithDeadline(ctx, losslessDeadline)
		}
		path, e := s.fetchCandidate(attemptCtx, t, c, p)
		cancelAttempt()
		if e != nil {
			failures = append(failures, c.Provider+": "+e.Error())
			s.record(t.ID, failures[len(failures)-1])
			continue
		}
		s.edit(t.ID, func(j *Task) { j.Phase = "التحقق من الصوت" })
		q, e := s.probe(ctx, path)
		if e == nil && p.LosslessOnly && !q.Lossless {
			e = errors.New("هذه النسخة مضغوطة؛ وضع Lossless فقط مفعّل")
		}
		if e == nil {
			e = s.verify(ctx, path)
		}
		if e != nil {
			failures = append(failures, c.Provider+": "+e.Error())
			s.record(t.ID, failures[len(failures)-1])
			continue
		}
		if t.Song.Duration > 0 && q.Duration > 0 {
			d := t.Song.Duration - q.Duration
			if d < 0 {
				d = -d
			}
			if d > 4 {
				failures = append(failures, c.Provider+": مدة الملف لا تطابق التسجيل")
				continue
			}
		}
		if !q.Lossless && p.AutoMatch && len(candidates) > 1 && c.Provider != t.Song.Provider {
			if losslessDeadline.IsZero() {
				losslessDeadline = time.Now().Add(20 * time.Second)
			}
			if bestPath == "" || q.Bitrate > bestQuality.Bitrate {
				hold := filepath.Join(s.root, "data", "partials", t.ID+"-best-lossy"+mediaExt(q.Codec))
				os.Remove(hold)
				if er := copyAtomic(path, hold); er != nil {
					return er
				}
				bestPath = hold
				bestQuality = q
				bestSource = c
			}
			s.record(t.ID, "نسخة مضغوطة متاحة؛ البحث عن نسخة Lossless مطابقة قبل اعتمادها")
			continue
		}
		return s.commitAudio(ctx, t, c, path, q, p)
	}
	if bestPath != "" {
		s.record(t.ID, "لم تتوفر نسخة Lossless مطابقة؛ اختيار أفضل نسخة مضغوطة متاحة")
		return s.commitAudio(ctx, t, bestSource, bestPath, bestQuality, p)
	}

	return errors.New(strings.Join(failures, "\n"))
}
func (s *Studio) commitAudio(ctx context.Context, t Task, c Song, path string, q Quality, p Preferences) error {
	if latest, ok := s.task(t.ID); ok {
		t.Song = latest.Song
	}
	var e error
	ext := mediaExt(q.Codec)
	name := safe(t.Song.Artist+" - "+t.Song.Title) + " [" + t.ID + "]"
	folder := filepath.Join(p.Folder, safe(t.Song.Artist), safe(t.Song.Album))
	if e = os.MkdirAll(folder, 0755); e != nil {
		return e
	}
	final := filepath.Join(folder, name+ext)
	if strings.ToLower(filepath.Ext(path)) != ext {
		dest := path + ".remux" + ext
		cmd := s.command(ctx, "ffmpeg", "-v", "error", "-nostdin", "-y", "-i", path, "-map", "0:a:0", "-c:a", "copy", dest)
		if out, er := cmd.CombinedOutput(); er != nil {
			return fmt.Errorf("تجهيز الحاوية: %s", trimError(out))
		}
		path = dest
	}
	if _, e = os.Stat(final); e == nil {
		return errors.New("يوجد ملف بنفس الاسم؛ لم يتم استبداله")
	}
	if e = copyAtomic(path, final); e != nil {
		return e
	}
	hash := fileHash(final)
	s.mu.Lock()
	duplicate := ""
	for _, j := range s.state.Jobs {
		if j.ID != t.ID && j.Hash == hash && hash != "" && j.Status == "done" {
			duplicate = j.Path
			break
		}
	}
	s.mu.Unlock()
	if duplicate != "" {
		os.Remove(final)
		final = duplicate
		s.record(t.ID, "الملف مطابق لملف في المكتبة؛ تم ربطه بدون حفظ نسخة ثانية")
	}
	s.edit(t.ID, func(j *Task) {
		j.Path = final
		j.Quality = q
		j.Status = "done"
		j.Phase = ""
		j.Hash = hash
		j.Source = c.Provider
		if info, e := os.Stat(final); e == nil {
			j.Total = info.Size()
			j.Bytes = info.Size()
		}
		if j.Song.Duration == 0 {
			j.Song.Duration = q.Duration
		}
	})
	s.queueMetadata(t.ID)
	s.cleanPartials(t.ID)
	s.record(t.ID, "تم التحقق وحفظ "+q.Codec)
	return nil
}

func copyAtomic(source, dest string) error {
	in, e := os.Open(source)
	if e != nil {
		return e
	}
	defer in.Close()
	tmp, e := os.CreateTemp(filepath.Dir(dest), ".madar-*")
	if e != nil {
		return e
	}
	name := tmp.Name()
	defer os.Remove(name)
	_, e = io.Copy(tmp, in)
	if e == nil {
		e = tmp.Sync()
	}
	ce := tmp.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	if _, e = os.Stat(dest); e == nil {
		return errors.New("الملف موجود")
	}
	return moveNew(name, dest)
}
