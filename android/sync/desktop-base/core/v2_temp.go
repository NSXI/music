package main

import (
	"os"
	"path/filepath"
	"strings"
)

func removeOwnedTemp(parent, path string) {
	base, e := filepath.Abs(parent)
	if e != nil {
		return
	}
	target, e := filepath.Abs(path)
	if e != nil {
		return
	}
	rel, e := filepath.Rel(base, target)
	if e != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return
	}
	os.RemoveAll(target)
}
