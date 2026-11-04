package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type LoudnessInfo struct {
	IntegratedLUFS float64 `json:"integratedLUFS"`
	TruePeakDB     float64 `json:"truePeakDB"`
	GainDB         float64 `json:"gainDB"`
	TargetLUFS     float64 `json:"targetLUFS"`
}

func (s *Studio) AnalyzeLoudness(id string) (LoudnessInfo, error) {
	t, ok := s.task(id)
	if !ok || t.Status != "done" {
		return LoudnessInfo{}, errors.New("اختر ملفًا محفوظًا")
	}
	p := filepath.Join(s.root, "data", "loudness", t.Hash+".json")
	if t.Hash == "" {
		p = filepath.Join(s.root, "data", "loudness", key(t.Path)+".json")
	}
	if b, e := os.ReadFile(p); e == nil {
		var v LoudnessInfo
		if json.Unmarshal(b, &v) == nil {
			return v, nil
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	release, e := s.acquireHeavyTask(ctx, "loudness")
	if e != nil {
		return LoudnessInfo{}, e
	}
	defer release()
	cmd := s.command(ctx, "ffmpeg", "-v", "info", "-nostdin", "-i", t.Path, "-map", "0:a:0", "-af", "loudnorm=I=-18:TP=-1:LRA=11:print_format=json", "-f", "null", "-")
	out := limitedBuffer{Limit: 64000}
	cmd.Stderr = &out
	if e = cmd.Run(); e != nil {
		return LoudnessInfo{}, errors.New("تعذر تحليل مستوى الصوت")
	}
	raw := out.String()
	at := strings.LastIndex(raw, "{")
	if at < 0 {
		return LoudnessInfo{}, errors.New("لم يرجع تحليل مستوى الصوت نتيجة")
	}
	var data map[string]string
	stop := strings.Index(raw[at:], "}")
	if stop < 0 {
		return LoudnessInfo{}, errors.New("نتيجة مستوى الصوت غير مكتملة")
	}
	if e = json.Unmarshal([]byte(raw[at:at+stop+1]), &data); e != nil {
		return LoudnessInfo{}, e
	}
	i, e1 := strconv.ParseFloat(data["input_i"], 64)
	peak, e2 := strconv.ParseFloat(data["input_tp"], 64)
	if e1 != nil || e2 != nil || math.IsNaN(i) || math.IsInf(i, 0) || math.IsNaN(peak) || math.IsInf(peak, 0) {
		return LoudnessInfo{}, errors.New("المقطع صامت أو لا يصلح لقياس مستوى الصوت")
	}
	v := LoudnessInfo{IntegratedLUFS: i, TruePeakDB: peak, TargetLUFS: -18, GainDB: math.Max(-24, math.Min(12, math.Min(-18-i, -1-peak)))}
	b, _ := json.Marshal(v)
	return v, writeChanged(p, b)
}
func (s *Studio) PreparePitchPlayback(id string, semitones float64) (Playback, error) {
	if math.IsNaN(semitones) || math.IsInf(semitones, 0) || semitones < -12 || semitones > 12 {
		return Playback{}, errors.New("طبقة الصوت يجب أن تكون بين -12 و12")
	}
	if semitones == 0 {
		return s.PreparePlayback(id)
	}
	t, ok := s.task(id)
	if !ok || t.Status != "done" || t.Song.Duration > 1800 {
		return Playback{}, errors.New("تغيير الطبقة المحلي يحتاج ملفًا محفوظًا حتى 30 دقيقة")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	release, e := s.acquireHeavyTask(ctx, "pitch")
	if e != nil {
		return Playback{}, e
	}
	defer release()
	q, e := s.probe(ctx, t.Path)
	if e != nil {
		return Playback{}, e
	}
	rate := q.Rate
	if rate <= 0 {
		rate = 48000
	}
	name := key(t.Hash + t.Path + fmt.Sprint(semitones))
	dir := filepath.Join(s.root, "data", "audio-fx")
	p := filepath.Join(dir, name+".wav")
	if st, e := os.Stat(p); e != nil || st.Size() < 44 {
		if e = os.MkdirAll(dir, 0700); e != nil {
			return Playback{}, e
		}
		factor := math.Pow(2, semitones/12)
		filter := fmt.Sprintf("asetrate=%.3f,aresample=%d,atempo=%.8f", float64(rate)*factor, rate, 1/factor)
		part := filepath.Join(dir, name+".partial.wav")
		cmd := s.command(ctx, "ffmpeg", "-v", "error", "-nostdin", "-y", "-i", t.Path, "-map", "0:a:0", "-af", filter, "-c:a", "pcm_f32le", part)
		if e = cmd.Run(); e != nil {
			os.Remove(part)
			return Playback{}, errors.New("تعذر تجهيز طبقة الصوت")
		}
		if e = os.Rename(part, p); e != nil {
			return Playback{}, e
		}
	}
	return Playback{URL: "/pitch/" + name, Quality: q, Decoded: true}, nil
}

type IntegrityResult struct {
	ID          string `json:"id"`
	Exists      bool   `json:"exists"`
	HashMatches bool   `json:"hashMatches"`
	Playable    bool   `json:"playable"`
	Message     string `json:"message"`
}

func (s *Studio) VerifyLibraryTrack(id string) (IntegrityResult, error) {
	t, ok := s.task(id)
	if !ok {
		return IntegrityResult{}, errors.New("المسار غير موجود")
	}
	r := IntegrityResult{ID: id}
	if _, e := os.Stat(t.Path); e != nil {
		r.Message = "الملف غير موجود؛ أعد ربط مجلده أو نزّله من جديد"
		return r, nil
	}
	r.Exists = true
	r.HashMatches = t.Hash != "" && fileHash(t.Path) == t.Hash
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if e := s.verify(ctx, t.Path); e != nil {
		r.Message = "فشل فحص الملف الصوتي"
		return r, nil
	}
	r.Playable = true
	if r.HashMatches {
		r.Message = "الملف سليم وبصمته مطابقة"
	} else {
		r.Message = "الصوت قابل للقراءة لكن الملف تغيّر منذ إضافته"
	}
	return r, nil
}
func (s *Studio) GetSearchGroups() ([][]Song, error) {
	s.mu.Lock()
	songs := make([]Song, 0, len(s.known))
	for _, v := range s.known {
		songs = append(songs, v)
	}
	s.mu.Unlock()
	if len(songs) > 2000 {
		return nil, errors.New("أعد البحث لتقليل النتائج")
	}
	return groupSearchSongs(songs), nil
}
