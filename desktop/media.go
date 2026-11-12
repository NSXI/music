package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	wr "github.com/wailsapp/wails/v2/pkg/runtime"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type limitedBuffer struct {
	bytes.Buffer
	Limit int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	remaining := b.Limit - b.Len()
	if remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		b.Buffer.Write(p)
	}
	return n, nil
}
func hiddenCommand(ctx context.Context, name string, args ...string) *exec.Cmd {
	c := exec.CommandContext(ctx, name, args...)
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return c
}
func (s *Studio) command(ctx context.Context, tool string, args ...string) *exec.Cmd {
	p := filepath.Join(s.root, "tools", tool+".exe")
	if _, e := os.Stat(p); e != nil {
		p = tool
	}
	if tool == "yt-dlp" {
		node := filepath.Join(s.root, "tools", "node.exe")
		if _, e := os.Stat(node); e == nil {
			args = append([]string{"--js-runtimes", "node:" + node}, args...)
		}
	}
	return hiddenCommand(ctx, p, args...)
}
func trimError(b []byte) string {
	v := regexp.MustCompile(`https?://\S+`).ReplaceAllString(string(b), "[رابط المصدر]")
	v = strings.TrimSpace(v)
	if len(v) > 1200 {
		v = v[len(v)-1200:]
	}
	if v == "" {
		v = "تعذر تنزيل الملف من هذا المصدر"
	}
	return v
}

type probeData struct {
	Streams []struct {
		Codec    string            `json:"codec_name"`
		Type     string            `json:"codec_type"`
		Bits     int               `json:"bits_per_sample"`
		RawBits  string            `json:"bits_per_raw_sample"`
		Rate     string            `json:"sample_rate"`
		Bitrate  string            `json:"bit_rate"`
		Channels int               `json:"channels"`
		Duration string            `json:"duration"`
		Tags     map[string]string `json:"tags"`
	} `json:"streams"`
	Format struct {
		Duration string            `json:"duration"`
		Bitrate  string            `json:"bit_rate"`
		Tags     map[string]string `json:"tags"`
	} `json:"format"`
}

func (s *Studio) probeDetails(ctx context.Context, path string) (Quality, probeData, error) {
	run, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, e := s.command(run, "ffprobe", "-v", "error", "-print_format", "json", "-show_format", "-show_streams", path).Output()
	if e != nil {
		return Quality{}, probeData{}, errors.New("الملف ليس صوتًا مقروءًا أو تالف")
	}
	var d probeData
	if e = json.Unmarshal(out, &d); e != nil {
		return Quality{}, d, e
	}
	for _, a := range d.Streams {
		if a.Type != "audio" {
			continue
		}
		q := Quality{Codec: strings.ToUpper(a.Codec), Bits: a.Bits, Channels: a.Channels}
		if n, _ := strconv.Atoi(a.RawBits); n > 0 {
			q.Bits = n
		}
		q.Rate, _ = strconv.Atoi(a.Rate)
		q.Bitrate, _ = strconv.Atoi(a.Bitrate)
		q.Duration, _ = strconv.ParseFloat(a.Duration, 64)
		if q.Duration <= 0 {
			q.Duration, _ = strconv.ParseFloat(d.Format.Duration, 64)
		}
		if q.Bitrate == 0 {
			q.Bitrate, _ = strconv.Atoi(d.Format.Bitrate)
		}
		q.Lossless = a.Codec == "flac" || a.Codec == "alac" || strings.HasPrefix(a.Codec, "pcm_") || a.Codec == "ape" || a.Codec == "wavpack" || a.Codec == "tta" || strings.HasPrefix(a.Codec, "dsd_")
		if q.Rate <= 0 || q.Duration <= 0 {
			return q, d, errors.New("معلومات الصوت غير مكتملة")
		}
		return q, d, nil
	}
	return Quality{}, d, errors.New("لا توجد قناة صوتية في الملف")
}
func (s *Studio) probe(ctx context.Context, path string) (Quality, error) {
	q, _, e := s.probeDetails(ctx, path)
	return q, e
}
func (s *Studio) verify(ctx context.Context, path string) error {
	run, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	var errText limitedBuffer
	errText.Limit = 2000
	c := s.command(run, "ffmpeg", "-v", "error", "-xerror", "-nostdin", "-i", path, "-map", "0:a:0", "-f", "null", "-")
	c.Stderr = &errText
	if e := c.Run(); e != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("فشل فحص فك الصوت: %s", trimError(errText.Bytes()))
	}
	return nil
}
func mediaExt(codec string) string {
	switch strings.ToLower(codec) {
	case "flac":
		return ".flac"
	case "alac", "aac":
		return ".m4a"
	case "mp3":
		return ".mp3"
	case "opus":
		return ".opus"
	case "vorbis":
		return ".ogg"
	case "ape":
		return ".ape"
	case "wavpack":
		return ".wv"
	case "ac3":
		return ".ac3"
	case "eac3":
		return ".eac3"
	default:
		if strings.HasPrefix(strings.ToLower(codec), "pcm_") {
			return ".wav"
		}
		return ".mka"
	}
}
func (s *Studio) enrich(ctx context.Context, id string) {
	t, ok := s.task(id)
	if !ok || t.Path == "" {
		return
	}
	if !t.MetadataPending {
		s.edit(id, func(j *Task) { j.Phase = "الغلاف والكلمات" })
	}
	base := strings.TrimSuffix(t.Path, filepath.Ext(t.Path))
	cover := t.Cover
	for _, ext := range []string{".jpg", ".jpeg", ".png"} {
		candidate := base + ext
		if _, err := readArtworkBytes(candidate); err == nil {
			cover = candidate
			break
		}
	}
	warning := []string{}
	if cover == "" && t.Song.Artwork != "" && validWebURL(t.Song.Artwork) == nil {
		run, cancel := context.WithTimeout(ctx, 15*time.Second)
		req, _ := http.NewRequestWithContext(run, "GET", t.Song.Artwork, nil)
		r, e := s.client.Do(req)
		if e == nil {
			b, er := io.ReadAll(io.LimitReader(r.Body, (8<<20)+1))
			r.Body.Close()
			if er == nil && r.StatusCode == 200 && len(b) <= 8<<20 {
				_, kind, er := image.DecodeConfig(bytes.NewReader(b))
				if er == nil {
					ext := ".jpg"
					if kind == "png" {
						ext = ".png"
					}
					cover = base + ".madar-source-" + key(string(b))[:12] + ext
					if artworkAtomicNew(cover, b) != nil {
						cover = ""
					}
				}
			}
		}
		cancel()
		if cover == "" {
			warning = append(warning, "تعذر حفظ غلاف المصدر")
		}
	}
	if cover == "" {
		candidate := filepath.Join(s.root, "data", "artwork", key(t.ID+"|"+t.Hash)+"-embedded.png")
		os.MkdirAll(filepath.Dir(candidate), 0755)
		if e := s.command(ctx, "ffmpeg", "-v", "error", "-nostdin", "-y", "-i", t.Path, "-map", "0:v:0", "-frames:v", "1", candidate).Run(); e == nil {
			cover = candidate
		}
	}
	if cover == "" && t.Song.Artist != "فنان غير معروف" {
		run, cancel := context.WithTimeout(ctx, 35*time.Second)
		if fetched, err := s.AutoArtwork(run, t); err == nil {
			cover = fetched
		}
		cancel()
	}
	if cover != "" && !t.Local && !t.MetadataPending {
		ext := strings.ToLower(filepath.Ext(t.Path))
		if ext == ".flac" || ext == ".mp3" || ext == ".m4a" {
			tagged := base + ".tagged" + ext
			args := []string{"-v", "error", "-nostdin", "-y", "-i", t.Path, "-i", cover, "-map", "0:a:0", "-map", "1:v:0", "-c", "copy", "-disposition:v", "attached_pic", "-metadata", "title=" + t.Song.Title, "-metadata", "artist=" + t.Song.Artist, "-metadata", "album=" + t.Song.Album, tagged}
			if e := s.command(ctx, "ffmpeg", args...).Run(); e == nil {
				if _, e = s.probe(ctx, tagged); e == nil {
					if e = os.Rename(tagged, t.Path); e != nil {
						warning = append(warning, "الغلاف محفوظ بجانب الصوت؛ تعذر دمجه")
					}
				}
			} else {
				warning = append(warning, "الغلاف محفوظ بجانب الصوت؛ لم ينجح الدمج")
			}
			os.Remove(tagged)
		}
	}
	lyrics := t.Lyrics
	if lyrics == "" {
		for _, ext := range []string{".lrc", ".txt"} {
			if st, e := os.Stat(base + ext); e == nil && st.Size() > 0 {
				lyrics = base + ext
				break
			}
		}
	}
	if lyrics == "" && t.Song.Artist != "" && t.Song.Artist != "فنان غير معروف" && s.GetState().Preferences.Lyrics {
		ly, e := s.remoteLyrics(ctx, t.Song)
		if e == nil {
			ext := ".txt"
			text := ly.Plain
			if ly.Synced != "" {
				ext = ".lrc"
				text = ly.Synced
			}
			if text != "" {
				lyrics = base + ext
				if os.WriteFile(lyrics, []byte(text), 0600) != nil {
					lyrics = ""
				}
			}
		} else {
			warning = append(warning, "لم تتوفر كلمات مطابقة")
		}
	}
	finalHash := fileHash(t.Path)
	s.edit(id, func(j *Task) {
		if j.Cover == t.Cover {
			j.Cover = cover
		}
		if j.Lyrics == t.Lyrics {
			j.Lyrics = lyrics
		}
		if cache, e := s.readLyricCache(t.Song); e == nil {
			j.LyricsSource = cache.Record.Provider
		}
		j.Warning = strings.Join(warning, "؛ ")
		j.Hash = finalHash
	})
	if lyrics != "" && !strings.HasSuffix(strings.ToLower(lyrics), ".lrc") && s.GetState().Preferences.AutoAlign {
		s.autoAlign(id)
	}
}

type LyricsRecord struct {
	Words        []WordLine `json:"-"`
	Provider     string     `json:"provider,omitempty"`
	Cached       bool       `json:"-"`
	Title        string     `json:"trackName"`
	Artist       string     `json:"artistName"`
	Album        string     `json:"albumName"`
	Duration     float64    `json:"duration"`
	Synced       string     `json:"syncedLyrics"`
	Plain        string     `json:"plainLyrics"`
	Instrumental bool       `json:"instrumental"`
}

func (s *Studio) remoteLyrics(ctx context.Context, song Song) (LyricsRecord, error) {
	return s.remoteLyricsFrom(ctx, song, "auto")
}

type LyricView struct {
	Pronunciation []string `json:"pronunciation,omitempty"`
	Source    string           `json:"source,omitempty"`
	Words     []WordLine       `json:"words,omitempty"`
	Alignment *AlignmentReport `json:"alignment,omitempty"`
	Text      string           `json:"text"`
	Timed     bool             `json:"timed"`
	Status    string           `json:"status"`
}

func (s *Studio) GetLyrics(id string, refresh bool) (LyricView, error) {
	t, ok := s.task(id)
	if !ok {
		return LyricView{}, errors.New("المسار غير موجود")
	}
	if !refresh {
		if cached, e := s.readLyricCache(t.Song); e == nil && (cached.Record.Synced != "" || cached.Record.Plain != "" || cached.Record.Instrumental) {
			r := cached.Record
			r.Cached = true
			return s.persistLyricView(t, r)
		}
		if t.Lyrics != "" {
			if b, e := os.ReadFile(t.Lyrics); e == nil && len(b) > 0 {
				if s.GetState().Preferences.AutoAlign && t.Path != "" {
					s.autoAlign(t.ID)
				}
				return LyricView{Text: string(b), Timed: validLRC(string(b), t.Song.Duration), Status: "ملف كلمات محفوظ محليًا", Source: t.LyricsSource}, nil
			}
		}
	}
	provider := "auto"
	if refresh {
		provider = "auto-refresh"
	}
	r, e := s.remoteLyricsFrom(context.Background(), t.Song, provider)
	if e != nil {
		if cached, er := s.readLyricCache(t.Song); er == nil {
			r = cached.Record
			r.Cached = true
			return s.persistLyricView(t, r)
		}
		return LyricView{}, e
	}
	return s.persistLyricView(t, r)
}

func (s *Studio) LoadLyricsFile(id string) error {
	if _, ok := s.task(id); !ok {
		return errors.New("اختر مسارًا")
	}
	path, e := wr.OpenFileDialog(s.ctx, wr.OpenDialogOptions{Title: "اختيار كلمات LRC / TTML / YRC / TXT", Filters: []wr.FileFilter{{DisplayName: "Lyrics", Pattern: "*.lrc;*.ttml;*.yrc;*.txt"}}})
	if e != nil || path == "" {
		return e
	}
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".lrc" && ext != ".txt" && ext != ".ttml" && ext != ".yrc" {
		return errors.New("اختر LRC / TTML / YRC / TXT")
	}
	b, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	return s.SaveImportedLyrics(id, string(b), ext)
}

func (s *Studio) ImportFiles() (int, error) {
	paths, e := wr.OpenMultipleFilesDialog(s.ctx, wr.OpenDialogOptions{Title: "أضف ملفات إلى مكتبتك", Filters: []wr.FileFilter{{DisplayName: "Audio", Pattern: "*.flac;*.mp3;*.m4a;*.aac;*.wav;*.ogg;*.opus;*.aiff;*.aif;*.wma;*.ape;*.wv;*.dsf;*.dff;*.ac3;*.eac3;*.mka;*.mp4;*.alac;*.tta;*.webm;*.amr;*.m4b;*.tak"}, {DisplayName: "All files", Pattern: "*.*"}}})
	if e != nil {
		return 0, e
	}
	return s.importPaths(context.Background(), paths)
}
func (s *Studio) importPaths(ctx context.Context, paths []string) (int, error) {
	s.importMu.Lock()
	defer s.importMu.Unlock()
	n := 0
	var failures []string
	for _, path := range paths {
		if ctx.Err() != nil {
			return n, ctx.Err()
		}
		q, d, e := s.probeDetails(ctx, path)
		if e != nil {
			failures = append(failures, filepath.Base(path))
			continue
		}
		h := fileHash(path)
		s.mu.Lock()
		found := false
		for i, j := range s.state.Jobs {
			if j.Hash == h && h != "" {
				if _, err:=os.Stat(j.Path); os.IsNotExist(err) {s.state.Jobs[i].Path=path;s.state.Jobs[i].Local=true;if info,e:=os.Stat(path);e==nil{s.state.Jobs[i].LocalModified=info.ModTime().UnixNano()}}
				if folderPathKey(j.Path) == folderPathKey(path) {
					if info, err := os.Stat(path); err == nil {
						s.state.Jobs[i].LocalModified = info.ModTime().UnixNano()
					}
				}
				found = true
				break
			}
		}
		s.mu.Unlock()
		if found {
			continue
		}
		tags := d.Format.Tags
		get := func(k string) string {
			for key, v := range tags {
				if strings.EqualFold(key, k) {
					return v
				}
			}
			return ""
		}
		title := get("title")
		if title == "" {
			title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		}
		artist := get("artist")
		if artist == "" {
			artist = "فنان غير معروف"
		}
		album := get("album")
		if album == "" {
			album = "ملفات محلية"
		}
		song := Song{Genres: genresFromProbe(d), ID: "local-" + key(path), Title: title, Artist: artist, Album: album, Duration: q.Duration, Provider: "local"}
		st, statErr := os.Stat(path)
		if statErr != nil {
			failures = append(failures, filepath.Base(path))
			continue
		}
		t := Task{LocalModified: st.ModTime().UnixNano(), ID: key(song.ID), Song: song, Status: "done", GenreChecked: true, Local: true, Source: "local", Path: path, Hash: h, Quality: q, Log: []string{"ملف أضفته من جهازك"}, Bytes: st.Size(), Total: st.Size()}
		for _, ext := range []string{".lrc", ".txt"} {
			sidecar := strings.TrimSuffix(path, filepath.Ext(path)) + ext
			if info, err := os.Stat(sidecar); err == nil && info.Size() > 0 && info.Size() <= 2<<20 {
				t.Lyrics = sidecar
				t.LyricsSource = "local"
				break
			}
		}
		s.mu.Lock()
		replaced := false
		for i, existing := range s.state.Jobs {
			if existing.Local && folderPathKey(existing.Path) == folderPathKey(path) {
				s.state.Jobs[i] = t
				replaced = true
				break
			}
		}
		if !replaced {
			s.state.Jobs = append(s.state.Jobs, t)
		}
		s.known[song.ID] = song
		s.save()
		s.mu.Unlock()
		if t.Lyrics == "" {
			for _, ext := range []string{".ttml", ".yrc"} {
				file := strings.TrimSuffix(path, filepath.Ext(path)) + ext
				if info, err := os.Stat(file); err == nil && info.Size() > 0 && info.Size() <= 2<<20 {
					if raw, err := os.ReadFile(file); err == nil && s.SaveImportedLyrics(t.ID, string(raw), ext) == nil {
						break
					}
				}
			}
		}
		s.queueMetadata(t.ID)
		s.edit(t.ID, func(j *Task) { j.Phase = "" })
		n++
	}
	s.mu.Lock()
	s.save()
	s.mu.Unlock()
	if len(failures) > 0 {
		return n, fmt.Errorf("تمت إضافة %d؛ تعذر قراءة: %s", n, strings.Join(failures, "، "))
	}
	return n, nil
}

type Playback struct {
	URL     string  `json:"url"`
	Quality Quality `json:"quality"`
	Decoded bool    `json:"decoded"`
}

func (s *Studio) PreparePlayback(id string) (Playback, error) { return s.preparePlayback(id, false) }
func (s *Studio) DecodePlayback(id string) (Playback, error)  { return s.preparePlayback(id, true) }
func (s *Studio) preparePlayback(id string, force bool) (Playback, error) {
	s.mediaMu.Lock()
	defer s.mediaMu.Unlock()
	t, ok := s.task(id)
	if !ok || t.Status != "done" {
		return Playback{}, errors.New("نزّل المسار أو أضف ملفًا محليًا أولًا")
	}
	t.Path = s.audioForPlayback(t)
	if _, e := os.Stat(t.Path); e != nil {
		return Playback{}, errors.New("الملف غير موجود في مكانه؛ أعد استيراده")
	}
	codec := strings.ToLower(t.Quality.Codec)
	native := codec == "flac" || codec == "mp3" || codec == "aac" || codec == "opus" || codec == "vorbis" || strings.HasPrefix(codec, "pcm_")
	ext := strings.ToLower(filepath.Ext(t.Path))
	nativeContainer := map[string]bool{".flac": true, ".mp3": true, ".aac": true, ".opus": true, ".ogg": true, ".wav": true, ".m4a": true, ".mp4": true, ".webm": true}[ext]
	if native && nativeContainer && !force {
		return Playback{URL: "/media/" + id, Quality: t.Quality}, nil
	}
	dir := filepath.Join(s.root, "data", "playback")
	os.MkdirAll(dir, 0755)
	dest := filepath.Join(dir, id+".wav")
	if _, e := os.Stat(dest); e != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		part := filepath.Join(dir, id+".partial.wav")
		out, e := s.command(ctx, "ffmpeg", "-v", "error", "-nostdin", "-y", "-i", t.Path, "-map", "0:a:0", "-c:a", "pcm_f32le", "-ac", "2", part).CombinedOutput()
		if e != nil {
			return Playback{}, errors.New(trimError(out))
		}
		if e = os.Rename(part, dest); e != nil {
			return Playback{}, e
		}
	}
	return Playback{URL: "/decoded/" + id, Quality: t.Quality, Decoded: true}, nil
}
func (s *Studio) MediaHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) != 2 {
			http.NotFound(w, r)
			return
		}
		if parts[0] == "stream" {
			s.servePreview(w, r, parts[1])
			return
		}
		if parts[0] == "artwork" {
			s.ServeArtwork(w, r, parts[1])
			return
		}
		if parts[0] == "pitch" {
			if len(parts[1])!=20 || strings.Trim(parts[1],"0123456789abcdef")!="" {http.NotFound(w,r);return}
			http.ServeFile(w,r,filepath.Join(s.root,"data","audio-fx",parts[1]+".wav"));return
		}
		t, ok := s.task(parts[1])
		if !ok {
			http.NotFound(w, r)
			return
		}
		path := ""
		switch parts[0] {
		case "media":
			path = s.audioForPlayback(t)
		case "art":
			path = t.Cover
		case "decoded":
			path = filepath.Join(s.root, "data", "playback", t.ID+".wav")
		}
		if path == "" {
			http.NotFound(w, r)
			return
		}
		if parts[0] == "art" && s.serveArtworkThumbnail(w, r, path) {
			return
		}
		w.Header().Set("Cache-Control", "private, max-age=60")
		http.ServeFile(w, r, path)
	})
}
func (s *Studio) Demo() (int, error) {
	dir := filepath.Join(s.root, "data", "demo")
	os.MkdirAll(dir, 0755)
	path := filepath.Join(dir, "Madar Soundcheck.flac")
	if _, e := os.Stat(path); e != nil {
		out, e := s.command(context.Background(), "ffmpeg", "-v", "error", "-nostdin", "-y", "-f", "lavfi", "-i", "aevalsrc=0.10*sin(2*PI*220*t)*sin(PI*mod(t\\,4)/4)+0.06*sin(2*PI*330*t)+0.04*sin(2*PI*440*t):s=96000:d=24", "-ac", "2", "-c:a", "flac", "-sample_fmt", "s32", "-metadata", "title=Madar Soundcheck", "-metadata", "artist=Madar Studio", "-metadata", "album=First light", path).CombinedOutput()
		if e != nil {
			return 0, errors.New(trimError(out))
		}
		text := "[00:00.00]هذا صوت مدار\n[00:04.00]تجربة محلية للمشغل\n[00:08.00]حرّك السرعة وجرّب الصدى\n[00:12.00]اختر شكل الكلمات الذي تحبه\n[00:16.00]موسيقاك، بطريقتك\n[00:20.00]مدار Music"
		os.WriteFile(strings.TrimSuffix(path, ".flac")+".lrc", []byte(text), 0600)
	}
	return s.importPaths(context.Background(), []string{path})
}
