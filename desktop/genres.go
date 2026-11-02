package main

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"unicode"
)

func cleanGenres(values []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		for _, g := range regexp.MustCompile(`[;,|]`).Split(value, -1) {
			g = strings.TrimSpace(strings.Map(func(r rune) rune {
				if unicode.IsControl(r) {
					return -1
				}
				return r
			}, g))
			if g == "" {
				continue
			}
			runes := []rune(g)
			if len(runes) > 60 {
				g = string(runes[:60])
			}
			id := strings.ToLower(g)
			if !seen[id] && len(out) < 12 {
				seen[id] = true
				out = append(out, g)
			}
		}
	}
	return out
}
func genresFromProbe(d probeData) []string {
	values := []string{}
	collect := func(tags map[string]string) {
		for k, v := range tags {
			if strings.EqualFold(k, "genre") {
				values = append(values, v)
			}
		}
	}
	collect(d.Format.Tags)
	for _, stream := range d.Streams {
		collect(stream.Tags)
	}
	return cleanGenres(values)
}
func (s *Studio) SetGenres(id string, genres []string) error {
	if len(genres) > 20 {
		return errors.New("أدخل حتى 12 تصنيفًا")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.state.Jobs {
		job := &s.state.Jobs[i]
		if job.ID == id {
			job.Song.Genres = cleanGenres(genres)
			job.GenreChecked = true
			s.known[job.Song.ID] = job.Song
			s.save()
			return nil
		}
	}
	return errors.New("المسار غير موجود في المكتبة")
}
func (s *Studio) refreshJobGenres(ctx context.Context, id string) {
	job, ok := s.task(id)
	if !ok || job.Path == "" || job.GenreChecked {
		return
	}
	_, d, e := s.probeDetails(ctx, job.Path)
	if e != nil {
		return
	}
	genres := genresFromProbe(d)
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.state.Jobs {
		t := &s.state.Jobs[i]
		if t.ID == id && !t.GenreChecked {
			if len(t.Song.Genres) == 0 {
				t.Song.Genres = genres
			}
			t.GenreChecked = true
			s.known[t.Song.ID] = t.Song
			s.save()
			return
		}
	}
}
func (s *Studio) refreshLibraryGenres(ctx context.Context) {
	for _, job := range s.GetState().Jobs {
		if ctx.Err() != nil {
			return
		}
		if job.Status == "done" && !job.GenreChecked {
			s.refreshJobGenres(ctx, job.ID)
		}
	}
}
