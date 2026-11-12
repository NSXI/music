package main

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
)

const continuityLimit = 32 << 20

type continuityConfig struct {
	Host  bool   `json:"host"`
	Port  int    `json:"port"`
	Token string `json:"token"`
	Peer  string `json:"peer"`
}

type ContinuityStatus struct {
	Hosting bool     `json:"hosting"`
	Paired  bool     `json:"paired"`
	Links   []string `json:"links"`
	Changes uint64   `json:"changes"`
}

type ContinuityPlayback struct {
	TaskID   string   `json:"taskId"`
	Title    string   `json:"title"`
	Position float64  `json:"position"`
	Queue    []string `json:"queue"`
	Paused   bool     `json:"paused"`
	TrackKey string   `json:"trackKey"`
}

type continuityRecord struct {
	Title  string      `json:"title"`
	Tracks []string    `json:"tracks,omitempty"`
	Lyrics *LyricCache `json:"lyrics,omitempty"`
}

type continuityRecords map[string]json.RawMessage

type continuityEdit struct {
	Key    string          `json:"key"`
	Before json.RawMessage `json:"before,omitempty"`
	After  json.RawMessage `json:"after,omitempty"`
}

type continuityRequest struct {
	Operation string           `json:"operation"`
	At        int64            `json:"at"`
	Track     string           `json:"track,omitempty"`
	Edits     []continuityEdit `json:"edits,omitempty"`
}

type continuityResponse struct {
	Records  continuityRecords  `json:"records,omitempty"`
	Playback ContinuityPlayback `json:"playback"`
	Applied  []string           `json:"applied,omitempty"`
	Error    string             `json:"error,omitempty"`
}

type ContinuitySyncResult struct {
	Received  int      `json:"received"`
	Sent      int      `json:"sent"`
	Missing   int      `json:"missing"`
	Conflicts []string `json:"conflicts"`
}

func (s *Studio) continuityConfigPath() string {
	return filepath.Join(s.root, "data", "continuity-v3.secure")
}

func (s *Studio) continuityConfig() (continuityConfig, error) {
	var cfg continuityConfig
	sealed, err := os.ReadFile(s.continuityConfigPath())
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	raw, err := protectSecret(sealed, true)
	if err == nil {
		err = json.Unmarshal(raw, &cfg)
	}
	return cfg, err
}

func (s *Studio) writeContinuityConfig(cfg continuityConfig) error {
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	sealed, err := protectSecret(raw, false)
	if err != nil {
		return err
	}
	return writeChanged(s.continuityConfigPath(), sealed)
}

func continuityAddresses(port int, token string) []string {
	links := []string{}
	addrs, _ := net.InterfaceAddrs()
	for _, address := range addrs {
		ip, _, err := net.ParseCIDR(address.String())
		if err == nil && ip.To4() != nil && ip.IsPrivate() && !ip.IsLoopback() {
			links = append(links, fmt.Sprintf("http://%s:%d/#%s", ip, port, token))
		}
	}
	return links
}

func (s *Studio) GetContinuityStatus() ContinuityStatus {
	s.v3Mu.Lock()
	defer s.v3Mu.Unlock()
	cfg, _ := s.continuityConfig()
	status := ContinuityStatus{Hosting: s.v3Host != nil, Paired: cfg.Peer != "", Changes: s.v3ContinuityChanges}
	if status.Hosting {
		status.Links = continuityAddresses(cfg.Port, cfg.Token)
	}
	return status
}

func (s *Studio) ResumeContinuity() error {
	s.v3Mu.Lock()
	defer s.v3Mu.Unlock()
	cfg, err := s.continuityConfig()
	if err != nil || !cfg.Host || s.v3Host != nil {
		return err
	}
	return s.listenContinuity(cfg, false)
}

func (s *Studio) EnableContinuityHost(enabled bool) error {
	s.v3Mu.Lock()
	defer s.v3Mu.Unlock()
	cfg, err := s.continuityConfig()
	if err != nil {
		return err
	}
	if !enabled {
		cfg.Host = false
		cfg.Token = ""
		cfg.Port = 0
		if err := s.writeContinuityConfig(cfg); err != nil {
			return err
		}
		if s.v3Host != nil {
			_ = s.v3Host.Close()
			s.v3Host = nil
		}
		return nil
	}
	if s.v3Host != nil {
		return nil
	}
	if !validPackID(cfg.Token) {
		cfg.Token, err = randomLinkToken()
		if err != nil {
			return err
		}
	}
	cfg.Host = true
	return s.listenContinuity(cfg, true)
}

func (s *Studio) listenContinuity(cfg continuityConfig, save bool) error {
	listener, err := net.Listen("tcp4", fmt.Sprintf("0.0.0.0:%d", cfg.Port))
	if err != nil {
		return err
	}
	cfg.Port = listener.Addr().(*net.TCPAddr).Port
	if save {
		if err = s.writeContinuityConfig(cfg); err != nil {
			listener.Close()
			return err
		}
	}
	aead, err := continuityCipher(cfg.Token)
	if err != nil {
		listener.Close()
		return err
	}
	replays := map[string]int64{}
	server := &http.Server{ReadHeaderTimeout: 4 * time.Second, ReadTimeout: 25 * time.Second, WriteTimeout: 40 * time.Second, MaxHeaderBytes: 4096}
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.serveContinuity(w, r, aead, replays)
	})
	s.v3Host = server
	go func() { _ = server.Serve(listener) }()
	return nil
}

func continuityCipher(token string) (cipher.AEAD, error) {
	if !validPackID(token) {
		return nil, errors.New("رمز الربط غير صالح")
	}
	key := sha256.Sum256([]byte("Nawa continuity 3:" + token))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func sealContinuity(aead cipher.AEAD, raw, context []byte) ([]byte, error) {
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return aead.Seal(nonce, nonce, raw, context), nil
}

func openContinuity(aead cipher.AEAD, data, context []byte) ([]byte, error) {
	n := aead.NonceSize()
	if len(data) < n+aead.Overhead() {
		return nil, errors.New("استجابة ربط غير صالحة")
	}
	return aead.Open(nil, data[:n], data[n:], context)
}

func (s *Studio) serveContinuity(w http.ResponseWriter, r *http.Request, aead cipher.AEAD, replays map[string]int64) {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	ip := net.ParseIP(host)
	if r.Method != "POST" || r.URL.Path != "/continuity-v3" || ip == nil || (!ip.IsPrivate() && !ip.IsLoopback()) {
		http.NotFound(w, r)
		return
	}
	packet, err := io.ReadAll(http.MaxBytesReader(w, r.Body, continuityLimit))
	if err != nil {
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		return
	}
	raw, err := openContinuity(aead, packet, []byte("request"))
	var query continuityRequest
	now := time.Now().Unix()
	if err != nil || json.Unmarshal(raw, &query) != nil || query.At < now-120 || query.At > now+120 {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	nonce := string(packet[:aead.NonceSize()])
	s.v3Mu.Lock()
	for key, at := range replays {
		if at < now-240 {
			delete(replays, key)
		}
	}
	if replays[nonce] != 0 || len(replays) >= 2048 {
		s.v3Mu.Unlock()
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}
	replays[nonce] = now
	s.v3Mu.Unlock()
	var response continuityResponse
	switch query.Operation {
	case "ping":
	case "state":
		response.Records, _, err = s.continuityRecords()
	case "playback":
		s.v3Mu.Lock()
		response.Playback = s.v3Playback
		s.v3Mu.Unlock()
	case "pause":
		s.v3Mu.Lock()
		if query.Track == s.v3Playback.TrackKey && query.Track != "" {
			s.v3PauseTrack = query.Track
		}
		s.v3Mu.Unlock()
	case "apply":
		response.Applied, err = s.applyContinuityEdits(query.Edits)
	default:
		err = errors.New("أمر ربط غير معروف")
	}
	if err != nil {
		response.Error = err.Error()
	}
	raw, _ = json.Marshal(response)
	if len(raw) > continuityLimit-128 {
		raw = []byte(`{"error":"مكتبة المزامنة أكبر من الحد المسموح"}`)
	}
	out, err := sealContinuity(aead, raw, packet[:aead.NonceSize()])
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(out)
}

func requestContinuity(link string, query continuityRequest) (continuityResponse, error) {
	var response continuityResponse
	base, token, err := parseDeviceLink(link)
	if err != nil {
		return response, err
	}
	aead, err := continuityCipher(token)
	if err != nil {
		return response, err
	}
	query.At = time.Now().Unix()
	raw, err := json.Marshal(query)
	if err != nil || len(raw) > continuityLimit-128 {
		return response, errors.New("بيانات الربط أكبر من الحد المسموح")
	}
	packet, err := sealContinuity(aead, raw, []byte("request"))
	if err != nil {
		return response, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", base+"/continuity-v3", bytes.NewReader(packet))
	if err != nil {
		return response, err
	}
	client := &http.Client{
		Transport:     &http.Transport{DisableKeepAlives: true},
		CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("Link redirects are not allowed") },
	}
	res, err := client.Do(req)
	if err != nil {
		return response, errors.New("الجهاز الآخر غير متصل")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return response, errors.New("تعذر التحقق من الربط، أو ساعة الجهازين مختلفة")
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, continuityLimit+1))
	if err != nil || len(body) > continuityLimit {
		return response, errors.New("استجابة ربط غير مكتملة")
	}
	raw, err = openContinuity(aead, body, packet[:aead.NonceSize()])
	if err != nil || json.Unmarshal(raw, &response) != nil {
		return response, errors.New("تعذر التحقق من استجابة الجهاز")
	}
	if response.Error != "" {
		return response, errors.New(response.Error)
	}
	return response, nil
}

func (s *Studio) PairContinuity(link string) error {
	link = strings.TrimSpace(link)
	if link != "" {
		if _, err := requestContinuity(link, continuityRequest{Operation: "ping"}); err != nil {
			return err
		}
	}
	s.v3SyncMu.Lock()
	defer s.v3SyncMu.Unlock()
	s.v3Mu.Lock()
	defer s.v3Mu.Unlock()
	cfg, err := s.continuityConfig()
	if err != nil {
		return err
	}
	cfg.Peer = link
	return s.writeContinuityConfig(cfg)
}

func (s *Studio) UpdateContinuityPlayback(id string, position float64, queue []string, paused bool) bool {
	s.v3Mu.Lock()
	defer s.v3Mu.Unlock()
	if s.v3Host == nil {
		return false
	}
	task, ok := s.task(id)
	if !ok || task.Status != "done" || !finiteTime(position) || len(queue) > 10000 {
		s.v3Playback = ContinuityPlayback{}
		return false
	}
	play := ContinuityPlayback{TrackKey: lyricKey(task.Song), Title: task.Song.Title, Position: position, Paused: paused}
	for _, id := range queue {
		if q, ok := s.task(id); ok && q.Status == "done" {
			play.Queue = append(play.Queue, lyricKey(q.Song))
		}
	}
	s.v3Playback = play
	pause := s.v3PauseTrack == play.TrackKey
	s.v3PauseTrack = ""
	return pause
}

func (s *Studio) GetContinuityPlayback() (ContinuityPlayback, error) {
	cfg, err := s.continuityConfig()
	if err != nil || cfg.Peer == "" {
		return ContinuityPlayback{}, errors.New("اربط الجهاز الآخر أولًا")
	}
	response, err := requestContinuity(cfg.Peer, continuityRequest{Operation: "playback"})
	if err != nil {
		return ContinuityPlayback{}, err
	}
	play := response.Playback
	tracks := s.continuityTracks()
	play.TaskID = tracks[play.TrackKey].ID
	queue := []string{}
	for _, k := range play.Queue {
		if local := tracks[k].ID; local != "" {
			queue = append(queue, local)
		}
	}
	play.Queue = queue
	return play, nil
}

func (s *Studio) ConfirmContinuityPlayback(track string) error {
	cfg, err := s.continuityConfig()
	if err != nil || cfg.Peer == "" {
		return errors.New("الجهاز الآخر غير مرتبط")
	}
	_, err = requestContinuity(cfg.Peer, continuityRequest{Operation: "pause", Track: track})
	return err
}

func (s *Studio) continuityTracks() map[string]Task {
	tracks := map[string]Task{}
	for _, task := range s.GetState().Jobs {
		if task.Status == "done" && task.UpgradeFor == "" {
			k := lyricKey(task.Song)
			if old := tracks[k]; old.ID == "" || task.Quality.Lossless && !old.Quality.Lossless {
				tracks[k] = task
			}
		}
	}
	return tracks
}

type continuityPlaylist struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Tracks []string `json:"tracks"`
}

func (s *Studio) continuityRecords() (continuityRecords, Workspace, error) {
	w, err := s.GetWorkspace()
	if err != nil {
		return nil, w, err
	}
	records := continuityRecords{}
	tracks := s.continuityTracks()
	ids := map[string]string{}
	for _, task := range s.GetState().Jobs {
		if task.Status == "done" && task.UpgradeFor == "" {
			ids[task.ID] = lyricKey(task.Song)
		}
	}
	var favorites []string
	_ = json.Unmarshal(w.UI["favorites"], &favorites)
	for _, id := range favorites {
		if k := ids[id]; k != "" {
			records["f/"+k] = json.RawMessage(`true`)
		}
	}
	var playlists []continuityPlaylist
	_ = json.Unmarshal(w.UI["playlists"], &playlists)
	for _, list := range playlists {
		rec := continuityRecord{Title: list.Name, Tracks: []string{}}
		for _, id := range list.Tracks {
			if k := ids[id]; k != "" {
				rec.Tracks = append(rec.Tracks, k)
			}
		}
		records["p/"+list.ID], _ = json.Marshal(rec)
	}
	for k, task := range tracks {
		c, err := s.readLyricCache(task.Song)
		if err != nil || !continuityLyric(c) {
			continue
		}
		c.Song = Song{}
		c.SavedAt = ""
		records["l/"+k], _ = json.Marshal(continuityRecord{Title: task.Song.Title, Lyrics: &c})
	}
	return records, w, nil
}

func continuityLyric(c LyricCache) bool {
	return c.Record.Provider == "manual" || c.Record.Provider == "local" || c.Alignment != nil || len(c.Pronunciation) > 0
}

func (s *Studio) applyContinuityEdits(edits []continuityEdit) ([]string, error) {
	s.v3ApplyMu.Lock()
	defer s.v3ApplyMu.Unlock()
	if len(edits) > 10000 {
		return nil, errors.New("عدد تعديلات كبير")
	}
	records, workspace, err := s.continuityRecords()
	if err != nil {
		return nil, err
	}
	tracks := s.continuityTracks()
	var favorites []string
	_ = json.Unmarshal(workspace.UI["favorites"], &favorites)
	var playlists []continuityPlaylist
	_ = json.Unmarshal(workspace.UI["playlists"], &playlists)
	applied := []string{}
	var lyricEdits []continuityEdit
	uiChanged := false
	for _, edit := range edits {
		kind, id, ok := strings.Cut(edit.Key, "/")
		if !ok || id == "" || len(id) > 200 || !bytes.Equal(records[edit.Key], edit.Before) {
			continue
		}
		switch kind {
		case "f":
			task := tracks[id]
			if task.ID == "" || (len(edit.After) > 0 && string(edit.After) != "true") {
				continue
			}
			favorites = slices.DeleteFunc(favorites, func(id string) bool {
				t, ok := s.task(id)
				return ok && lyricKey(t.Song) == edit.Key[2:]
			})
			if len(edit.After) > 0 {
				favorites = append(favorites, task.ID)
			}
			uiChanged = true
		case "p":
			if len(edit.After) == 0 {
				playlists = slices.DeleteFunc(playlists, func(p continuityPlaylist) bool { return p.ID == id })
			} else {
				var record continuityRecord
				if json.Unmarshal(edit.After, &record) != nil || len(record.Title) > 1000 || len(record.Tracks) > 10000 {
					continue
				}
				list := continuityPlaylist{ID: id, Name: record.Title, Tracks: []string{}}
				for _, k := range record.Tracks {
					if local := tracks[k].ID; local != "" {
						list.Tracks = append(list.Tracks, local)
					}
				}
				if len(list.Tracks) != len(record.Tracks) {
					continue
				}
				index := slices.IndexFunc(playlists, func(p continuityPlaylist) bool { return p.ID == id })
				if index >= 0 {
					playlists[index] = list
				} else {
					playlists = append(playlists, list)
				}
			}
			uiChanged = true
		case "l":
			lyricEdits = append(lyricEdits, edit)
			continue
		default:
			continue
		}
		applied = append(applied, edit.Key)
	}
	if uiChanged {
		workspace.UI["favorites"], _ = json.Marshal(append([]string{}, favorites...))
		workspace.UI["playlists"], _ = json.Marshal(append([]continuityPlaylist{}, playlists...))
		if _, err = s.SaveWorkspace(workspace.Revision, workspace.UI, workspace.Downloads); err != nil {
			return nil, err
		}
	}
	for _, edit := range lyricEdits {
		task := tracks[strings.TrimPrefix(edit.Key, "l/")]
		var record continuityRecord
		if task.ID == "" || json.Unmarshal(edit.After, &record) != nil || record.Lyrics == nil {
			continue
		}
		cache := record.Lyrics
		if !continuityLyric(*cache) || len(cache.Record.Plain)+len(cache.Record.Synced) > 2<<20 ||
			len(cache.Words) > 0 && validateEditedLines(cache.Words, task.Song.Duration) != nil {
			continue
		}
		if old, err := s.readLyricCache(task.Song); err == nil {
			if continuityLyric(old) {
				old.Song = Song{}
				old.SavedAt = ""
				actual, _ := json.Marshal(continuityRecord{Title: task.Song.Title, Lyrics: &old})
				if !bytes.Equal(actual, edit.Before) {
					continue
				}
			}
			if err := s.backupLyricEdit(task, old); err != nil {
				continue
			}
		}
		cache.Song = task.Song
		cache.SavedAt = time.Now().UTC().Format(time.RFC3339)
		if err := s.writeLyricCache(*cache); err == nil {
			applied = append(applied, edit.Key)
		}
	}
	if len(applied) > 0 {
		s.v3Mu.Lock()
		s.v3ContinuityChanges++
		s.v3Mu.Unlock()
	}
	return applied, nil
}

func (s *Studio) SyncContinuity(choice string) (ContinuitySyncResult, error) {
	s.v3SyncMu.Lock()
	defer s.v3SyncMu.Unlock()
	var result ContinuitySyncResult
	if choice != "review" && choice != "local" && choice != "peer" {
		return result, errors.New("خيار مزامنة غير صالح")
	}
	cfg, err := s.continuityConfig()
	if err != nil || cfg.Peer == "" {
		return result, errors.New("اربط الجهاز الآخر أولًا")
	}
	peer, err := requestContinuity(cfg.Peer, continuityRequest{Operation: "state"})
	if err != nil {
		return result, err
	}
	local, _, err := s.continuityRecords()
	if err != nil {
		return result, err
	}
	_, token, _ := parseDeviceLink(cfg.Peer)
	path := filepath.Join(s.root, "data", "continuity-"+key(token)+".json")
	base := continuityRecords{}
	if raw, err := os.ReadFile(path); err == nil && len(raw) <= continuityLimit {
		_ = json.Unmarshal(raw, &base)
	}
	if base == nil {
		base = continuityRecords{}
	}
	all := map[string]bool{}
	for _, records := range []continuityRecords{base, local, peer.Records} {
		for k := range records {
			all[k] = true
		}
	}
	var incoming, outgoing []continuityEdit
	for k := range all {
		before, here, there := base[k], local[k], peer.Records[k]
		switch {
		case bytes.Equal(here, there):
			base[k] = here
		case bytes.Equal(here, before):
			incoming = append(incoming, continuityEdit{k, here, there})
		case bytes.Equal(there, before):
			outgoing = append(outgoing, continuityEdit{k, there, here})
		case choice == "local":
			outgoing = append(outgoing, continuityEdit{k, there, here})
		case choice == "peer":
			incoming = append(incoming, continuityEdit{k, here, there})
		default:
			var record continuityRecord
			_ = json.Unmarshal(here, &record)
			if record.Title == "" {
				_ = json.Unmarshal(there, &record)
			}
			if record.Title == "" {
				record.Title = k
			}
			result.Conflicts = append(result.Conflicts, record.Title)
		}
	}
	if len(incoming) > 0 {
		applied, err := s.applyContinuityEdits(incoming)
		if err != nil {
			return result, err
		}
		result.Received = len(applied)
		for _, k := range applied {
			base[k] = peer.Records[k]
		}
	}
	if len(outgoing) > 0 {
		applied, err := requestContinuity(cfg.Peer, continuityRequest{Operation: "apply", Edits: outgoing})
		if err != nil {
			return result, err
		}
		result.Sent = len(applied.Applied)
		for _, k := range applied.Applied {
			base[k] = local[k]
		}
	}
	for k, record := range base {
		if len(record) == 0 {
			delete(base, k)
		}
	}
	raw, err := json.Marshal(base)
	if err != nil {
		return result, err
	}
	if len(raw) > continuityLimit {
		return result, errors.New("بيانات المزامنة أكبر من الحد المسموح")
	}
	if err := writeChanged(path, raw); err != nil {
		return result, err
	}
	sort.Strings(result.Conflicts)
	result.Missing = len(incoming) + len(outgoing) - result.Received - result.Sent
	return result, nil
}
