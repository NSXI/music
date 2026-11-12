package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ClipStatus struct {
	ID      string `json:"id"`
	Running bool   `json:"running"`
	Ready   bool   `json:"ready"`
	Frames  int    `json:"frames"`
	Error   string `json:"error,omitempty"`
}

type clipBuild struct {
	dir, track    string
	start, length float64
	width, height int
	elapsed       float64
	bytes         int64
	durations     []float64
	encoding      bool
}

func (s *Studio) clipRoot() string {
	return filepath.Join(s.root, "data", "lyric-clips")
}

func (s *Studio) BeginLyricClip(track string, start, end float64, vertical bool) (string, error) {
	task, ok := s.task(track)
	if !ok || task.Status != "done" || !finiteTime(start) || !finiteTime(end) ||
		end-start < 1 || end-start > 90 || end > task.Song.Duration+.1 {
		return "", errors.New("اختر مقطعًا بين ثانية و90 ثانية من أغنية محفوظة")
	}
	s.v3Mu.Lock()
	defer s.v3Mu.Unlock()
	if s.v3Clip.Running {
		return "", errors.New("يجري تصدير مقطع آخر")
	}
	id, err := randomLinkToken()
	if err != nil {
		return "", err
	}
	root := s.clipRoot()
	if err := os.MkdirAll(root, 0700); err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp(root, ".render-")
	if err != nil {
		return "", err
	}
	width, height := 960, 540
	if vertical {
		width, height = 540, 960
	}
	s.v3ClipBuild = &clipBuild{dir: dir, track: track, start: start, length: end - start, width: width, height: height}
	s.v3Clip = ClipStatus{ID: id, Running: true}
	return id, nil
}

func (s *Studio) AddLyricClipFrame(id string, seconds float64, encoded string) error {
	if !finiteTime(seconds) || seconds <= 0 || len(encoded) > 3<<20 {
		return errors.New("إطار التصدير غير صالح")
	}
	s.v3Mu.Lock()
	defer s.v3Mu.Unlock()
	b := s.v3ClipBuild
	if b == nil || b.encoding || id != s.v3Clip.ID || len(b.durations) >= 300 || b.elapsed+seconds > b.length+.025 {
		return errors.New("جلسة تصدير غير صالحة")
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || b.bytes+int64(len(raw)) > 64<<20 {
		return errors.New("حجم إطارات المقطع أكبر من الحد")
	}
	picture, err := png.DecodeConfig(bytes.NewReader(raw))
	if err != nil || picture.Width != b.width || picture.Height != b.height {
		return errors.New("أبعاد إطار الفيديو غير صالحة")
	}
	if err := os.WriteFile(filepath.Join(b.dir, fmt.Sprintf("%04d.png", len(b.durations))), raw, 0600); err != nil {
		return err
	}
	b.durations = append(b.durations, seconds)
	b.elapsed += seconds
	b.bytes += int64(len(raw))
	s.v3Clip.Frames = len(b.durations)
	return nil
}

func (s *Studio) FinishLyricClip(id string) error {
	s.v3Mu.Lock()
	defer s.v3Mu.Unlock()
	b := s.v3ClipBuild
	if b == nil || b.encoding || id != s.v3Clip.ID || len(b.durations) == 0 || math.Abs(b.elapsed-b.length) > .025 {
		return errors.New("إطارات الفيديو غير مكتملة")
	}
	task, ok := s.task(b.track)
	if !ok || task.Status != "done" {
		return errors.New("ملف الصوت لم يعد متاحًا")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	s.v3ClipCancel = cancel
	b.encoding = true
	go func() {
		defer cancel()
		err := s.encodeLyricClip(ctx, id, b, task)
		removeOwnedTemp(s.clipRoot(), b.dir)
		s.v3Mu.Lock()
		defer s.v3Mu.Unlock()
		s.v3Clip.Running = false
		s.v3Clip.Ready = err == nil
		s.v3ClipCancel = nil
		s.v3ClipBuild = nil
		if err != nil {
			s.v3Clip.Error = err.Error()
		}
	}()
	return nil
}

func (s *Studio) encodeLyricClip(ctx context.Context, id string, b *clipBuild, task Task) error {
	release, err := s.acquireHeavyTask(ctx, "lyric-video")
	if err != nil {
		return err
	}
	defer release()
	var list strings.Builder
	for i, duration := range b.durations {
		fmt.Fprintf(&list, "file '%04d.png'\nduration %.6f\n", i, duration)
	}
	fmt.Fprintf(&list, "file '%04d.png'\n", len(b.durations)-1)
	frames := filepath.Join(b.dir, "frames.txt")
	if err := os.WriteFile(frames, []byte(list.String()), 0600); err != nil {
		return err
	}
	output := filepath.Join(b.dir, "video.mp4")
	cmd := s.command(ctx, "ffmpeg", "-v", "error", "-nostdin", "-y", "-f", "concat", "-safe", "1", "-i", frames,
		"-ss", formatSeconds(b.start), "-i", s.audioForPlayback(task), "-map", "0:v:0", "-map", "1:a:0",
		"-t", formatSeconds(b.length), "-vf", "fps=30,format=yuv420p", "-c:v", "mpeg4", "-q:v", "3",
		"-threads", "2", "-c:a", "aac", "-b:a", "160k", "-movflags", "+faststart", output)
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("تعذر ترميز فيديو الكلمات")
	}
	return os.Rename(output, filepath.Join(s.clipRoot(), id+".mp4"))
}

func (s *Studio) GetLyricClipStatus() ClipStatus {
	s.v3Mu.Lock()
	defer s.v3Mu.Unlock()
	return s.v3Clip
}

func (s *Studio) CancelLyricClip() {
	s.v3Mu.Lock()
	defer s.v3Mu.Unlock()
	if s.v3ClipCancel != nil {
		s.v3ClipCancel()
		return
	}
	if b := s.v3ClipBuild; b != nil {
		removeOwnedTemp(s.clipRoot(), b.dir)
		s.v3ClipBuild = nil
		s.v3Clip.Running = false
	}
}

func (s *Studio) LyricClipPath(id string) (string, error) {
	if !validPackID(id) {
		return "", errors.New("معرّف فيديو غير صالح")
	}
	path := filepath.Join(s.clipRoot(), id+".mp4")
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
		return "", errors.New("مقطع الكلمات غير جاهز")
	}
	return path, nil
}

func (s *Studio) HasV3BackgroundWork() bool {
	s.v3Mu.Lock()
	defer s.v3Mu.Unlock()
	return s.v3Offline.Running || s.v3Clip.Running
}
