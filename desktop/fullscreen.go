package main

import (
	"errors"
	wr "github.com/wailsapp/wails/v2/pkg/runtime"
)

// Full-screen lyrics uses the native monitor-sized window, rather than a
// maximised content pane. Wails restores the prior window placement on exit.
func (s *Studio) SetLyricsFullscreen(enabled bool) error {
	if s.ctx == nil {
		return errors.New("شغّل نافذة التطبيق لاستخدام ملء الشاشة")
	}
	s.screenMu.Lock()
	defer s.screenMu.Unlock()
	if enabled {
		if !wr.WindowIsFullscreen(s.ctx) {
			s.fullscreenWasMaximised = wr.WindowIsMaximised(s.ctx)
			wr.WindowFullscreen(s.ctx)
		}
	} else {
		if wr.WindowIsFullscreen(s.ctx) {
			wr.WindowUnfullscreen(s.ctx)
			if s.fullscreenWasMaximised {
				wr.WindowMaximise(s.ctx)
			}
		}
	}
	return nil
}
