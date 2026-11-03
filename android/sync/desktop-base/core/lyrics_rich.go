package main

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

var yrcLine = regexp.MustCompile(`^\[(\d+),(\d+)\]`)
var yrcWord = regexp.MustCompile(`\((\d+),(\d+),\d+\)([^\(]*)`)

func decodeYRC(raw string, song Song) (LyricsRecord, error) {
	record := LyricsRecord{Title: song.Title, Artist: song.Artist, Album: song.Album, Duration: song.Duration}
	if len(raw) > 2<<20 {
		return record, errors.New("YRC exceeds limit")
	}
	var lrc, plain strings.Builder
	for _, row := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		head := yrcLine.FindStringSubmatch(row)
		if len(head) != 3 {
			continue
		}
		start, _ := strconv.ParseFloat(head[1], 64)
		length, _ := strconv.ParseFloat(head[2], 64)
		start /= 1000
		end := start + length/1000
		if length <= 0 || song.Duration > 0 && end > song.Duration+2 {
			return record, errors.New("YRC timing exceeds recording")
		}
		line := WordLine{Start: start, End: end}
		var text strings.Builder
		for _, match := range yrcWord.FindAllStringSubmatch(row[len(head[0]):], -1) {
			at, _ := strconv.ParseFloat(match[1], 64)
			duration, _ := strconv.ParseFloat(match[2], 64)
			at /= 1000
			stop := at + duration/1000
			if duration <= 0 || at < start-.001 || stop > end+.001 || len(line.Words) > 0 && at < line.Words[len(line.Words)-1].Start {
				return record, errors.New("YRC word timing is invalid")
			}
			text.WriteString(match[3])
			value := strings.TrimSpace(match[3])
			if value != "" {
				line.Words = append(line.Words, TimedWord{Text: value, Start: at, End: stop})
			}
		}
		line.Text = strings.TrimSpace(text.String())
		if line.Text == "" || len(line.Words) == 0 {
			continue
		}
		record.Words = append(record.Words, line)
		ms := int64(math.Round(start * 1000))
		fmt.Fprintf(&lrc, "[%02d:%02d.%03d]%s\n", ms/60000, ms/1000%60, ms%1000, line.Text)
		plain.WriteString(line.Text + "\n")
	}
	if !validLRC(lrc.String(), song.Duration) {
		return record, errors.New("لا يوجد توقيت YRC صالح لهذا التسجيل")
	}
	record.Synced = lrc.String()
	record.Plain = strings.TrimSpace(plain.String())
	return record, nil
}
func (s *Studio) SaveImportedLyrics(id, text, format string) error {
	task, ok := s.task(id)
	if !ok {
		return errors.New("اختر ملفًا صوتيًا محفوظًا")
	}
	var record LyricsRecord
	var err error
	switch strings.ToLower(strings.TrimPrefix(format, ".")) {
	case "ttml":
		record, err = decodeBetterTTML(text, task.Song)
	case "yrc":
		record, err = decodeYRC(text, task.Song)
	case "lrc":
		return s.SaveLyricsText(id, text, true)
	default:
		return s.SaveLyricsText(id, text, false)
	}
	if err != nil {
		return err
	}
	if record.Synced == "" {
		return errors.New("ملف الكلمات لا يحتوي توقيتًا صالحًا")
	}
	s.translationMu.Lock()
	defer s.translationMu.Unlock()
	record.Provider = "local"
	if err = s.writeLyricCache(LyricCache{Song: task.Song, Record: record, Words: record.Words}); err != nil {
		return err
	}
	_, err = s.persistLyricView(task, record)
	return err
}
