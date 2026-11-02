package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

var thumbnailMu sync.Mutex

func readArtworkCachedBytes(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, artworkMaxBytes+1))
	if err != nil || len(raw) > artworkMaxBytes {
		return nil, fmt.Errorf("invalid cached image size")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > artworkMaxPixels {
		return nil, fmt.Errorf("invalid cached image dimensions")
	}
	return raw, nil
}
func thumbnailSize(request *http.Request) int {
	size, _ := strconv.Atoi(request.URL.Query().Get("size"))
	switch size {
	case 128, 256, 512, 1024:
		return size
	}
	return 0
}
func (s *Studio) serveArtworkThumbnail(w http.ResponseWriter, r *http.Request, path string) bool {
	size := thumbnailSize(r)
	if size == 0 {
		return false
	}
	info, err := os.Stat(path)
	if err != nil {
		http.NotFound(w, r)
		return true
	}
	identity := key(fmt.Sprintf("v1|%s|%d|%d|%d", path, info.Size(), info.ModTime().UnixNano(), size))
	dir := filepath.Join(s.root, "data", "artwork", "thumbnails")
	target := filepath.Join(dir, identity+".image")
	thumbnailMu.Lock()
	if _, err = os.Stat(target); err != nil {
		raw, readErr := readArtworkCachedBytes(path)
		if readErr != nil {
			thumbnailMu.Unlock()
			http.NotFound(w, r)
			return true
		}
		source, format, decodeErr := image.Decode(bytes.NewReader(raw))
		if decodeErr != nil {
			thumbnailMu.Unlock()
			http.NotFound(w, r)
			return true
		}
		bounds := source.Bounds()
		scale := math.Min(1, float64(size)/float64(max(bounds.Dx(), bounds.Dy())))
		width, height := max(1, int(float64(bounds.Dx())*scale)), max(1, int(float64(bounds.Dy())*scale))
		small := image.NewRGBA(image.Rect(0, 0, width, height))
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				fx, fy := (float64(x)+.5)/scale-.5, (float64(y)+.5)/scale-.5
				x0, y0 := int(math.Floor(fx)), int(math.Floor(fy))
				ax, ay := fx-float64(x0), fy-float64(y0)
				sample := func(px, py int) (float64, float64, float64, float64) {
					rr, gg, bb, aa := source.At(bounds.Min.X+min(bounds.Dx()-1, max(0, px)), bounds.Min.Y+min(bounds.Dy()-1, max(0, py))).RGBA()
					return float64(rr), float64(gg), float64(bb), float64(aa)
				}
				r0, g0, b0, a0 := sample(x0, y0)
				r1, g1, b1, a1 := sample(x0+1, y0)
				r2, g2, b2, a2 := sample(x0, y0+1)
				r3, g3, b3, a3 := sample(x0+1, y0+1)
				mix := func(a, b, c, d float64) uint8 {
					return uint8(math.Round(((a*(1-ax)+b*ax)*(1-ay) + (c*(1-ax)+d*ax)*ay) / 257))
				}
				small.SetRGBA(x, y, color.RGBA{R: mix(r0, r1, r2, r3), G: mix(g0, g1, g2, g3), B: mix(b0, b1, b2, b3), A: mix(a0, a1, a2, a3)})
			}
		}
		var output bytes.Buffer
		if format == "jpeg" {
			err = jpeg.Encode(&output, small, &jpeg.Options{Quality: 84})
		} else {
			encoder := png.Encoder{CompressionLevel: png.BestSpeed}
			err = encoder.Encode(&output, small)
		}
		if err == nil {
			err = os.MkdirAll(dir, 0755)
		}
		if err == nil {
			err = artworkAtomicNew(target, output.Bytes())
		}
		if err != nil {
			thumbnailMu.Unlock()
			http.NotFound(w, r)
			return true
		}
	}
	thumbnailMu.Unlock()
	w.Header().Set("Cache-Control", "private, max-age=86400, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeFile(w, r, target)
	return true
}
