package main

import "testing"

func TestExtractorPreferredArtwork(t *testing.T) {
	v := &extractorInfo{ID: "jNQXAC9IVRw", Thumbnail: "https://images.example/valid.jpg"}
	v.Thumbnails = append(v.Thumbnails, struct {
		URL   string `json:"url"`
		Width int    `json:"width"`
	}{URL: "https://images.example/missing-maxres.webp"})
	if extractorArtwork("ytmusic", v) != v.Thumbnail {
		t.Fatal("speculative image replaced the validated cover")
	}
	v.Thumbnail = ""
	if extractorArtwork("ytmusic", v) != "https://i.ytimg.com/vi/jNQXAC9IVRw/hqdefault.jpg" {
		t.Fatal("missing safe video-thumbnail fallback")
	}
}
