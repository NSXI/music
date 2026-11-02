package main

import (
	"context"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Clear DASH manifests are copied without converting the audio codec.
// No decryption keys or protected-media license requests are implemented.
func (s *Studio) saveClearDASH(data []byte) (string, error) {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	root := false
	for {
		token, e := decoder.Token()
		if e == io.EOF {
			break
		}
		if e != nil {
			return "", e
		}
		if element, ok := token.(xml.StartElement); ok {
			if !root {
				if element.Name.Local != "MPD" {
					return "", errors.New("المصدر لم يرجع بيان DASH صالحًا")
				}
				root = true
			}
			if strings.EqualFold(element.Name.Local, "ContentProtection") {
				return "", errors.New("بث محمي؛ تتم محاولة مصدر بديل")
			}
			for _, attr := range element.Attr {
				if attr.Name.Local == "initialization" || attr.Name.Local == "media" {
					if strings.Contains(attr.Value, "://") && validWebURL(attr.Value) != nil {
						return "", errors.New("رابط غير صالح في بيان الصوت")
					}
				}
			}
		}
	}
	if !root {
		return "", errors.New("بيان فارغ")
	}
	dir := filepath.Join(s.root, "data", "manifests")
	if e := os.MkdirAll(dir, 0755); e != nil {
		return "", e
	}
	path := filepath.Join(dir, key(string(data))+".mpd")
	if e := os.WriteFile(path, data, 0600); e != nil {
		return "", e
	}
	return "dash:" + path, nil
}
func (s *Studio) fetchClearDASH(ctx context.Context, t Task, address string) (string, error) {
	path := strings.TrimPrefix(address, "dash:")
	if filepath.Dir(path) != filepath.Join(s.root, "data", "manifests") {
		return "", errors.New("بيان الصوت خارج مسار التطبيق")
	}
	dir := filepath.Join(s.root, "data", "partials")
	os.MkdirAll(dir, 0755)
	dest := filepath.Join(dir, t.ID+"-dash.mka")
	out, e := s.command(ctx, "ffmpeg", "-v", "error", "-nostdin", "-y", "-protocol_whitelist", "file,http,https,tcp,tls", "-i", path, "-map", "0:a:0", "-c:a", "copy", dest).CombinedOutput()
	if e != nil {
		return "", errors.New(trimError(out))
	}
	return dest, nil
}
