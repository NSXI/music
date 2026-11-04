package main

import (
	"errors"
	wr "nawa.android/core/desktop"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (s *Studio) ExportTextDocument(name, text string) (string, error) {
	if len(text) > 64<<20 || name == "" || filepath.Base(name) != name || strings.ContainsAny(name, `/\:`) {
		return "", errors.New("اسم أو حجم ملف غير صالح")
	}
	dir, e := wr.OpenDirectoryDialog(s.ctx, wr.OpenDialogOptions{Title: "اختر مجلد التصدير"})
	if e != nil || dir == "" {
		return "", e
	}
	ext := filepath.Ext(name)
	name = strings.TrimSuffix(name, ext) + "-" + time.Now().Format("20060102-150405") + ext
	p := filepath.Join(dir, name)
	f, e := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return "", e
	}
	_, e = f.WriteString(text)
	ce := f.Close()
	if e == nil {
		e = ce
	}
	return name, e
}
func (s *Studio) ImportTextDocument() (string, error) {
	p, e := wr.OpenFileDialog(s.ctx, wr.OpenDialogOptions{Title: "اختر ملف الكلمات أو الثيم أو النسخة الاحتياطية", Filters: []wr.FileFilter{{DisplayName: "Nawa documents", Pattern: "*.json;*.lrc;*.ttml;*.yrc;*.txt"}}})
	if e != nil || p == "" {
		return "", e
	}
	st, e := os.Stat(p)
	if e != nil {
		return "", e
	}
	if st.Size() > 64<<20 {
		return "", errors.New("حجم الملف أكبر من 64 MB")
	}
	b, e := os.ReadFile(p)
	return string(b), e
}
