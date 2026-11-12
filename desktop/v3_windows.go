package main

import (
	"errors"
	"os"
	"path/filepath"

	wr "github.com/wailsapp/wails/v2/pkg/runtime"
)

func (s *Studio) SetMiniPlayer(enabled bool) error {
	if s.ctx == nil {
		return errors.New("شغّل نافذة التطبيق أولًا")
	}
	s.screenMu.Lock()
	defer s.screenMu.Unlock()
	if wr.WindowIsFullscreen(s.ctx) {
		return errors.New("اخرج من ملء الشاشة لفتح المشغّل المصغر")
	}
	if enabled {
		if s.v3MiniSize[0] > 0 {
			return nil
		}
		s.v3MiniWasMaximized = wr.WindowIsMaximised(s.ctx)
		if s.v3MiniWasMaximized {
			wr.WindowUnmaximise(s.ctx)
		}
		width, height := wr.WindowGetSize(s.ctx)
		s.v3MiniSize = [2]int{width, height}
		wr.WindowSetMinSize(s.ctx, 320, 112)
		wr.WindowSetSize(s.ctx, 470, 138)
		wr.WindowSetAlwaysOnTop(s.ctx, true)
	} else if s.v3MiniSize[0] > 0 {
		wr.WindowSetAlwaysOnTop(s.ctx, false)
		wr.WindowSetMinSize(s.ctx, 960, 680)
		wr.WindowSetSize(s.ctx, s.v3MiniSize[0], s.v3MiniSize[1])
		if s.v3MiniWasMaximized {
			wr.WindowMaximise(s.ctx)
		}
		s.v3MiniSize = [2]int{}
	}
	return nil
}

func (s *Studio) SaveLyricClip(id string) (string, error) {
	source, err := s.LyricClipPath(id)
	if err != nil {
		return "", err
	}
	if s.ctx == nil {
		return "", errors.New("شغّل نافذة التطبيق للتصدير")
	}
	destination, err := wr.SaveFileDialog(s.ctx, wr.SaveDialogOptions{
		Title: "حفظ فيديو الكلمات", DefaultFilename: "Nawa-lyrics.mp4",
		Filters: []wr.FileFilter{{DisplayName: "MP4 video", Pattern: "*.mp4"}},
	})
	if err != nil || destination == "" {
		return "", err
	}
	if filepath.Ext(destination) == "" {
		destination += ".mp4"
	}
	if err := copyAtomic(source, destination); err != nil {
		return "", err
	}
	_ = os.Remove(source)
	return destination, nil
}
