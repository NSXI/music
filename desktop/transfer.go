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
	"strconv"
	"strings"
	"time"
)

func retryDelay(r *http.Response) time.Duration {
	if n, e := strconv.Atoi(r.Header.Get("Retry-After")); e == nil && n >= 0 {
		return time.Duration(n) * time.Second
	}
	if d, e := http.ParseTime(r.Header.Get("Retry-After")); e == nil {
		if time.Until(d) > 0 {
			return time.Until(d)
		}
	}
	return 5 * time.Second
}
func (s *Studio) waitRetry(ctx context.Context, id string, d time.Duration) error {
	s.edit(id, func(t *Task) { t.RetryAt = time.Now().Add(d).UnixMilli(); t.Phase = "بانتظار المصدر" })
	s.mu.Lock()
	s.save()
	s.mu.Unlock()
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
	}
	s.edit(id, func(t *Task) { t.RetryAt = 0; t.Phase = "تنزيل الصوت" })
	return nil
}
func (s *Studio) transfer(ctx context.Context, t Task, address string) (string, error) {
	if e := validWebURL(address); e != nil {
		return "", e
	}
	dir := filepath.Join(s.root, "data", "partials")
	if e := os.MkdirAll(dir, 0755); e != nil {
		return "", e
	}
	part := filepath.Join(dir, t.ID+".part")
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if e := s.sourceCooldown(ctx, t.ID, address); e != nil {
			return "", e
		}
		latest, _ := s.task(t.ID)
		offset := int64(0)
		if latest.TransferURL == address && latest.ETag != "" && !strings.HasPrefix(latest.ETag, "W/") {
			if st, e := os.Stat(part); e == nil {
				offset = st.Size()
			}
		}
		if latest.TransferURL != address {
			s.edit(t.ID, func(j *Task) { j.TransferURL = address; j.ETag = ""; j.Bytes = 0; j.Total = 0 })
			s.mu.Lock()
			s.save()
			s.mu.Unlock()
		}
		req, _ := http.NewRequestWithContext(ctx, "GET", address, nil)
		req.Header.Set("User-Agent", "MadarMusic/1.0")
		req.Header.Set("Accept-Encoding", "identity")
		if offset > 0 {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
			req.Header.Set("If-Range", latest.ETag)
		}
		client := &http.Client{Transport: s.client.Transport}
		res, e := client.Do(req)
		if e != nil {
			last = e
			if e = s.waitRetry(ctx, t.ID, time.Duration(attempt+1)*time.Second); e != nil {
				return "", e
			}
			continue
		}
		if res.StatusCode == 429 || res.StatusCode >= 500 {
			last = fmt.Errorf("HTTP %d", res.StatusCode)
			delay := retryDelay(res)
			if res.StatusCode == 429 {
				s.setSourceCooldown(address, delay)
			}
			res.Body.Close()
			if attempt < 2 {
				if e = s.waitRetry(ctx, t.ID, delay); e != nil {
					return "", e
				}
			}
			continue
		}
		if res.StatusCode == 416 {
			res.Body.Close()
			os.Remove(part)
			s.edit(t.ID, func(j *Task) { j.ETag = "" })
			last = errors.New("تغيّر حجم المصدر؛ بدء نسخة جديدة")
			continue
		}
		if res.StatusCode != 200 && res.StatusCode != 206 {
			res.Body.Close()
			return "", fmt.Errorf("HTTP %d", res.StatusCode)
		}
		total := res.ContentLength
		etag := res.Header.Get("ETag")
		if res.StatusCode == 206 {
			var start, end, all int64
			_, er := fmt.Sscanf(res.Header.Get("Content-Range"), "bytes %d-%d/%d", &start, &end, &all)
			if er != nil || offset == 0 || start != offset || end < start || all <= end || (res.ContentLength >= 0 && res.ContentLength != end-start+1) || (etag != "" && etag != latest.ETag) {
				res.Body.Close()
				os.Remove(part)
				return "", errors.New("نطاق استكمال غير صالح؛ لم يتم دمج الملفات")
			}
			total = all
		} else {
			offset = 0
		}
		ct := strings.ToLower(res.Header.Get("Content-Type"))
		if strings.Contains(ct, "text/html") || strings.Contains(ct, "application/json") {
			res.Body.Close()
			return "", errors.New("المصدر أعاد صفحة أو رسالة بدل ملف صوتي")
		}
		flags := os.O_CREATE | os.O_WRONLY
		if offset > 0 {
			flags |= os.O_APPEND
		} else {
			flags |= os.O_TRUNC
		}
		f, e := os.OpenFile(part, flags, 0600)
		if e != nil {
			res.Body.Close()
			return "", e
		}
		s.edit(t.ID, func(j *Task) { j.TransferURL = address; j.ETag = etag; j.Total = total; j.Bytes = offset })
		s.mu.Lock()
		s.save()
		s.mu.Unlock()
		buf := make([]byte, 128*1024)
		n := offset
		lastUpdate := time.Now()
		transferStarted := time.Now()
		limit := s.downloadPolicy().BytesPerSecond
		var re error
		for {
			if ctx.Err() != nil {
				re = ctx.Err()
				break
			}
			count, readErr := res.Body.Read(buf)
			if count > 0 {
				written, we := f.Write(buf[:count])
				n += int64(written)
				if we != nil {
					re = we
					break
				}
				if written != count {
					re = io.ErrShortWrite
					break
				}
				if limit > 0 {
					delay := time.Duration(float64(n-offset)/float64(limit)*float64(time.Second))-time.Since(transferStarted)
					if delay > 0 { timer:=time.NewTimer(delay);select {case <-ctx.Done():timer.Stop();re=ctx.Err();case <-timer.C:};if re!=nil{break} }
				}
			}
			if time.Since(lastUpdate) > 200*time.Millisecond {
				s.edit(t.ID, func(j *Task) { j.Bytes = n })
				lastUpdate = time.Now()
			}
			if readErr != nil {
				if readErr != io.EOF {
					re = readErr
				}
				break
			}
		}
		if re == nil {
			re = f.Sync()
		}
		ce := f.Close()
		res.Body.Close()
		if re == nil {
			re = ce
		}
		s.edit(t.ID, func(j *Task) { j.Bytes = n })
		if re == nil && total >= 0 && n != total {
			re = io.ErrUnexpectedEOF
		}
		if re == nil {
			s.edit(t.ID, func(j *Task) { j.Total = n; j.Bytes = n })
			return part, nil
		}
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		last = re
		if e = s.waitRetry(ctx, t.ID, time.Duration(attempt+1)*time.Second); e != nil {
			return "", e
		}
	}
	return "", fmt.Errorf("تعذر إكمال الملف بعد 3 محاولات: %w", last)
}
func (s *Studio) extractDownload(ctx context.Context, t Task, song Song, p Preferences) (string, error) {
	dir := filepath.Join(s.root, "data", "partials", t.ID)
	if e := os.MkdirAll(dir, 0755); e != nil {
		return "", e
	}
	selectors := []string{"best[ext=flac]/best[ext=wav]/download/bestaudio/best", "bestaudio/best"}
	if song.Provider == "bandcamp" {
		selectors = []string{"bestaudio/best"}
	}
	if p.LosslessOnly {
		selectors = []string{"best[ext=flac]/best[ext=wav]/bestaudio[acodec=alac]/download"}
	}
	var last string
	for attempt, selector := range selectors {
		prefix := fmt.Sprintf("media-%d", attempt)
		pattern := filepath.Join(dir, prefix+".%(ext)s")
		args := []string{"--ignore-config", "--no-plugin-dirs", "--no-cache-dir", "--no-playlist", "--socket-timeout", "20", "--retries", "2", "--fragment-retries", "2", "--no-warnings", "--newline", "--no-color", "--progress-template", "download:%(progress)j", "--ffmpeg-location", filepath.Join(s.root, "tools"), "--fixup", "never", "-f", selector, "-o", strings.ReplaceAll(pattern, "%", "%%"), "--", song.URL}
		args = append([]string{"--write-info-json"}, args...)
		if limit:=s.downloadPolicy().BytesPerSecond;limit>0 { args=append([]string{"--limit-rate",strconv.FormatInt(limit,10)},args...) }
		// Keep the output extension template; escape only literal percent signs in its parent path.
		for i, v := range args {
			if v == strings.ReplaceAll(pattern, "%", "%%") {
				args[i] = strings.ReplaceAll(dir, "%", "%%") + string(os.PathSeparator) + prefix + ".%(ext)s"
			}
		}
		cmd := s.command(ctx, "yt-dlp", args...)
		stdout, e := cmd.StdoutPipe()
		if e != nil {
			return "", e
		}
		var stderr limitedBuffer
		stderr.Limit = 5000
		cmd.Stderr = &stderr
		if e = cmd.Start(); e != nil {
			return "", e
		}
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 4096), 1<<20)
		for scanner.Scan() {
			var v struct {
				Bytes    int64 `json:"downloaded_bytes"`
				Total    int64 `json:"total_bytes"`
				Estimate int64 `json:"total_bytes_estimate"`
			}
			line := strings.TrimPrefix(scanner.Text(), "download:")
			if json.Unmarshal([]byte(line), &v) == nil {
				if v.Total <= 0 {
					v.Total = v.Estimate
				}
				s.edit(t.ID, func(j *Task) { j.Bytes = v.Bytes; j.Total = v.Total })
			}
		}
		readErr := scanner.Err()
		e = cmd.Wait()
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if e == nil && readErr == nil {
			// Reuse metadata already obtained for this download, avoiding another
			// extractor process or a guess from the search result's filename.
			if raw, er := os.ReadFile(filepath.Join(dir, prefix+".info.json")); er == nil && len(raw) <= 32<<20 && song.Provider == t.Song.Provider {
				var info extractorInfo
				if json.Unmarshal(raw, &info) == nil {
					s.edit(t.ID, func(j *Task) {
						if info.Track != "" {
							j.Song.Title = info.Track
						}
						if info.Artist != "" {
							j.Song.Artist = info.Artist
						} else if j.Song.Artist == "" {
							j.Song.Artist = info.Uploader
						}
						if info.Album != "" {
							j.Song.Album = info.Album
						}
						if info.Duration > 0 {
							j.Song.Duration = info.Duration
						}
						if art := extractorArtwork(song.Provider, &info); art != "" {
							j.Song.Artwork = art
						}
					})
				}
			}
			files, _ := filepath.Glob(filepath.Join(dir, prefix+".*"))
			for _, f := range files {
				if strings.HasSuffix(f, ".part") || strings.HasSuffix(f, ".ytdl") || strings.HasSuffix(f, ".info.json") {
					continue
				}
				if st, er := os.Stat(f); er == nil && st.Size() > 0 {
					return f, nil
				}
			}
		}
		last = trimError(stderr.Bytes())
		s.record(t.ID, song.Provider+": تعذرت النسخة المطلوبة، الانتقال إلى البديل المتاح")
	}
	return "", errors.New(last)
}
