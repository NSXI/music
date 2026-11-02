package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"image"
	"image/color"
	"image/png"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWorkerBudgetSerializesAndCancels(t *testing.T) {
	s := testStudio(t)
	release, err := s.acquireHeavyTask(context.Background(), "translation")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		unlock, e := s.acquireHeavyTask(ctx, "alignment")
		if unlock != nil {
			unlock()
		}
		done <- e
	}()
	select {
	case <-done:
		t.Fatal("two heavy models acquired a slot")
	case <-time.After(30 * time.Millisecond):
	}
	cancel()
	if err := <-done; err == nil {
		t.Fatal("waiting task ignored cancellation")
	}
	release()
	release, err = s.acquireHeavyTask(context.Background(), "alignment")
	if err != nil {
		t.Fatal(err)
	}
	release()
	release()
	if s.GetState().Resource.ActiveHeavy != "" {
		t.Fatal("resource slot was not cleared")
	}
	if boundedAffinityMask(0b101101, 2) != 0b000101 || boundedAffinityMask(0, 2) != 0 {
		t.Fatal("invalid core budget mask")
	}
	if workerCores("economy") != 2 || workerCores("balanced") != 4 || workerCores("smooth") != 6 {
		t.Fatal("invalid worker budgets")
	}
	if err := s.SetResourceMode("invalid"); err == nil {
		t.Fatal("invalid resource mode accepted")
	}
	if err := s.SetResourceMode("smooth"); err != nil {
		t.Fatal(err)
	}
	if NewStudio(s.root).GetState().Preferences.ResourceMode != "smooth" {
		t.Fatal("mode did not persist")
	}
}
func TestBudgetAppliedToOwnedHiddenWorker(t *testing.T) {
	s := testStudio(t)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	cmd := hiddenCommand(ctx, "powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", "Start-Sleep -Milliseconds 600")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	state := s.applyWorkerBudget(cmd)
	err := cmd.Wait()
	if err != nil {
		t.Fatal(err)
	}
	if !state.LowPriorityApplied {
		t.Fatal("background priority was not applied to own worker")
	}
	if !state.AffinityApplied {
		t.Skip("processor group policy does not permit an affinity mask on this machine")
	}
	if state.RequestedCores != 2 {
		t.Fatal("economy did not request two cores")
	}
}
func TestArtworkThumbnailPreservesOriginalAndCaches(t *testing.T) {
	s := testStudio(t)
	source := image.NewRGBA(image.Rect(0, 0, 1600, 900))
	for y := 0; y < 900; y++ {
		for x := 0; x < 1600; x++ {
			source.SetRGBA(x, y, color.RGBA{R: uint8(x % 255), G: uint8(y % 255), B: 120, A: 255})
		}
	}
	var imageBytes bytes.Buffer
	png.Encode(&imageBytes, source)
	path := filepath.Join(s.root, "original.png")
	os.WriteFile(path, imageBytes.Bytes(), 0600)
	before := sha256.Sum256(imageBytes.Bytes())
	request := httptest.NewRequest("GET", "/art/fixture?size=512", nil)
	recorder := httptest.NewRecorder()
	if !s.serveArtworkThumbnail(recorder, request, path) {
		t.Fatal("thumbnail not served")
	}
	if recorder.Code != 200 {
		t.Fatalf("thumbnail HTTP%d", recorder.Code)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(recorder.Body.Bytes()))
	if err != nil || cfg.Width != 512 || cfg.Height != 288 {
		t.Fatalf("invalid thumbnail %+v %v", cfg, err)
	}
	raw, _ := os.ReadFile(path)
	if sha256.Sum256(raw) != before {
		t.Fatal("original artwork was changed")
	}
	files, _ := os.ReadDir(filepath.Join(s.root, "data", "artwork", "thumbnails"))
	if len(files) != 1 {
		t.Fatal("thumbnail cache missing")
	}
	again := httptest.NewRecorder()
	s.serveArtworkThumbnail(again, request, path)
	if !bytes.Equal(again.Body.Bytes(), recorder.Body.Bytes()) {
		t.Fatal("cache did not reuse identical bytes")
	}
	bad := filepath.Join(s.root, "bad.png")
	os.WriteFile(bad, []byte("<html>not an image</html>"), 0600)
	invalid := httptest.NewRecorder()
	s.serveArtworkThumbnail(invalid, request, bad)
	if invalid.Code != 404 {
		t.Fatal("invalid image rendered")
	}
}
