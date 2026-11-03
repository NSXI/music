package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var amllIndexMu sync.Mutex
var amllFileName = regexp.MustCompile(`^[A-Za-z0-9_-]+\.ttml$`)

func init() {
	lyricProviderNames["amll"] = "AMLL · TTML كلمات/مقاطع"
	defaultLyricProviders = append([]string{"amll"}, defaultLyricProviders...)
}

// Contributor-authored timing, retrieved on demand; no bundled lyric corpus.
func (s *Studio) amllIndex(ctx context.Context) (string, error) {
	amllIndexMu.Lock()
	defer amllIndexMu.Unlock()
	folder := filepath.Join(s.root, "data", "lyrics-index")
	file := filepath.Join(folder, "amll.jsonl")
	if info, err := os.Stat(file); err == nil && time.Since(info.ModTime()) < 12*time.Hour {
		return file, nil
	}
	base := s.metadataBase("amll", "https://raw.githubusercontent.com/amll-dev/amll-ttml-db/main")
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, base+"/metadata/raw-lyrics-index.jsonl", nil)
	request.Header.Set("User-Agent", "NawaMusic (https://github.com/NSXI/music)")
	response, err := s.client.Do(request)
	if err != nil {
		if _, old := os.Stat(file); old == nil {
			return file, nil
		}
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return "", fmt.Errorf("AMLL index HTTP %d", response.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (3<<20)+1))
	if err != nil || len(raw) > 3<<20 {
		return "", errors.New("AMLL index exceeds limit")
	}
	if len(raw) == 0 || !json.Valid([]byte(strings.SplitN(string(raw), "\n", 2)[0])) {
		return "", errors.New("AMLL index is invalid")
	}
	if err = os.MkdirAll(folder, 0755); err != nil {
		return "", err
	}
	temp, err := os.CreateTemp(folder, ".amll-index-*")
	if err != nil {
		return "", err
	}
	name := temp.Name()
	defer os.Remove(name)
	if _, err = temp.Write(raw); err != nil {
		temp.Close()
		return "", err
	}
	temp.Close()
	if err = os.Rename(name, file); err != nil {
		return "", err
	}
	return file, nil
}
func (s *Studio) fetchAMLL(ctx context.Context, song Song) (LyricsRecord, error) {
	empty := LyricsRecord{}
	index, err := s.amllIndex(ctx)
	if err != nil {
		return empty, err
	}
	file, err := os.Open(index)
	if err != nil {
		return empty, err
	}
	defer file.Close()
	type entry struct {
		Metadata [][]json.RawMessage `json:"metadata"`
		File     string              `json:"rawLyricFile"`
	}
	var selected []entry
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 128<<10)
	for scanner.Scan() {
		if ctx.Err() != nil {
			return empty, ctx.Err()
		}
		var item entry
		if json.Unmarshal(scanner.Bytes(), &item) != nil || !amllFileName.MatchString(item.File) {
			continue
		}
		metadata := map[string][]string{}
		for _, pair := range item.Metadata {
			if len(pair) != 2 {
				continue
			}
			var key string
			var values []string
			if json.Unmarshal(pair[0], &key) == nil && json.Unmarshal(pair[1], &values) == nil {
				metadata[key] = values
			}
		}
		titles := metadata["musicName"]
		artists := metadata["artists"]
		titleMatch, artistMatch := false, false
		for _, value := range titles {
			if norm(value) == norm(song.Title) {
				titleMatch = true
			}
		}
		for _, value := range artists {
			if norm(value) == norm(song.Artist) {
				artistMatch = true
			}
		}
		exactID := false
		if song.Provider == "spotify" && song.Ref != "" {
			for _, value := range metadata["spotifyId"] {
				if value == song.Ref {
					exactID = true
				}
			}
		}
		if exactID {
			selected = []entry{item}
			break
		}
		if titleMatch && artistMatch {
			selected = append(selected, item)
			if len(selected) > 4 {
				break
			}
		}
	}
	if scanner.Err() != nil {
		return empty, scanner.Err()
	}
	if len(selected) == 0 {
		return empty, errors.New("لا يوجد TTML يطابق العنوان والفنان")
	}
	if len(selected) > 1 {
		return empty, errors.New("عدة تسجيلات TTML متشابهة؛ يلزم معرف التسجيل أو ملف تختاره")
	}
	base := s.metadataBase("amll", "https://raw.githubusercontent.com/amll-dev/amll-ttml-db/main")
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, base+"/raw-lyrics/"+selected[0].File, nil)
	response, err := s.client.Do(request)
	if err != nil {
		return empty, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return empty, fmt.Errorf("AMLL TTML HTTP %d", response.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (3<<20)+1))
	if err != nil {
		return empty, err
	}
	return decodeBetterTTML(string(raw), song)
}
