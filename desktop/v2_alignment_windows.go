package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

func (s *Studio) computeRangeAlignment(ctx context.Context, audio, text, language string, duration float64) (alignmentOutput, error) {
	dir, e := os.MkdirTemp(filepath.Join(s.root, "data", "range-alignment"), "model-")
	if e != nil {
		return alignmentOutput{}, e
	}
	defer removeOwnedTemp(filepath.Join(s.root, "data", "range-alignment"), dir)
	output := filepath.Join(dir, "result.json")
	request := map[string]any{"audio": audio, "text": text, "language": language, "model": filepath.Join(s.root, "tools", "models", "base.pt"), "output": output, "ffmpeg": filepath.Join(s.root, "tools", "ffmpeg.exe")}
	raw, _ := json.Marshal(request)
	input := filepath.Join(dir, "input.json")
	if e = os.WriteFile(input, raw, 0600); e != nil {
		return alignmentOutput{}, e
	}
	cmd := hiddenCommand(ctx, s.alignmentWorker(), input)
	cmd.Env = withWorkerEnvironment(append(os.Environ(), "HF_HUB_OFFLINE=1", "TRANSFORMERS_OFFLINE=1"), s.backgroundWorkerEnv())
	stderr := limitedBuffer{Limit: 2000}
	cmd.Stderr = &stderr
	if e = cmd.Start(); e != nil {
		return alignmentOutput{}, e
	}
	s.applyWorkerBudget(cmd)
	if e = cmd.Wait(); e != nil {
		return alignmentOutput{}, errors.New("لم يكتمل تحليل النطاق؛ الكلمات الأصلية محفوظة")
	}
	raw, e = os.ReadFile(output)
	if e != nil {
		return alignmentOutput{}, e
	}
	var result alignmentOutput
	e = json.Unmarshal(raw, &result)
	result.Original = text
	return result, e
}

func(s *Studio) missingAlignmentMessage()string{return "محرك التوقيت غير جاهز. افتح الإعدادات ونزّل مكوّن التوقيت المحلي."}
