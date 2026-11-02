package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func (s *Studio) cleanPartials(id string) {
	if !regexp.MustCompile(`^[0-9a-f]{20}$`).MatchString(id) {
		return
	}
	dir := filepath.Join(s.root, "data", "partials")
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		if entry.Name() == id && entry.IsDir() {
			target := filepath.Join(dir, entry.Name())
			rel, e := filepath.Rel(dir, target)
			if e == nil && rel == id {
				os.RemoveAll(target)
			}
		} else if !entry.IsDir() && (strings.HasPrefix(entry.Name(), id+".") || strings.HasPrefix(entry.Name(), id+"-")) {
			os.Remove(filepath.Join(dir, entry.Name()))
		}
	}
	s.edit(id, func(t *Task) { t.TransferURL = ""; t.ETag = "" })
}
