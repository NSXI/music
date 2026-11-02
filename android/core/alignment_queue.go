package main

import (
	"math"
	"strings"
)

func (s *Studio) queueAlignment(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closing {
		return
	}
	for _, old := range s.state.AlignmentQueue {
		if old == id {
			return
		}
	}
	s.state.AlignmentQueue = append(s.state.AlignmentQueue, id)
	s.save()
}
func (s *Studio) nextAlignment() {
	for {
		s.mu.Lock()
		if s.closing || s.alignState.Running || len(s.state.AlignmentQueue) == 0 || !s.state.Preferences.AutoAlign {
			s.mu.Unlock()
			return
		}
		id := s.state.AlignmentQueue[0]
		s.state.AlignmentQueue = s.state.AlignmentQueue[1:]
		language := s.state.Preferences.AlignmentLanguage
		s.save()
		s.mu.Unlock()
		if s.alignmentReady() {
			if e := s.StartAlignment(id, language); e == nil {
				return
			}
		}
	}
}

// A line timestamp alone is not word alignment, including partially timed sources.
func wordTimingComplete(rows []WordLine, text string, duration float64) bool {
	var original strings.Builder
	count := 0
	previous := -1.0
	for _, row := range rows {
		for _, word := range row.Words {
			if math.IsNaN(word.Start) || math.IsNaN(word.End) || math.IsInf(word.Start, 0) || math.IsInf(word.End, 0) || word.Start < 0 || word.End <= word.Start || word.Start < previous || (duration > 0 && word.End > duration+.1) {
				return false
			}
			original.WriteString(word.Text)
			previous = word.Start
			count++
		}
	}
	return count > 0 && alignmentIdentity(original.String()) == alignmentIdentity(stripLyricText(text))
}
