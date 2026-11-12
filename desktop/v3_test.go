package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestV3PairedPlaybackAndFavoriteUseLocalTrackID(t *testing.T) {
	host, peer := testStudio(t), testStudio(t)
	hostTrack := v2Task(t, host)
	peerTrack := v2Task(t, peer)
	peerTrack.ID = "other-device-track"
	peer.state.Jobs[0] = peerTrack
	if err := host.EnableContinuityHost(true); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = host.EnableContinuityHost(false) })
	cfg, err := host.continuityConfig()
	if err != nil {
		t.Fatal(err)
	}
	link := fmt.Sprintf("http://127.0.0.1:%d/#%s", cfg.Port, cfg.Token)
	if err = peer.PairContinuity(link); err != nil {
		t.Fatal(err)
	}
	sealed, err := os.ReadFile(peer.continuityConfigPath())
	if err != nil || bytes.Contains(sealed, []byte(cfg.Token)) {
		t.Fatal("pairing token was exposed or not saved", err)
	}
	host.UpdateContinuityPlayback(hostTrack.ID, 3.25, []string{hostTrack.ID}, false)
	play, err := peer.GetContinuityPlayback()
	if err != nil || play.TaskID != peerTrack.ID || play.Position != 3.25 || len(play.Queue) != 1 || play.Queue[0] != peerTrack.ID {
		t.Fatal("playback was not mapped to the local copy", err)
	}
	if err = peer.ConfirmContinuityPlayback(play.TrackKey); err != nil || !host.UpdateContinuityPlayback(hostTrack.ID, 3.3, nil, false) {
		t.Fatal("source did not receive acknowledgement", err)
	}
	workspace, err := host.GetWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	workspace.UI["favorites"], _ = json.Marshal([]string{hostTrack.ID})
	if _, err = host.SaveWorkspace(workspace.Revision, workspace.UI, workspace.Downloads); err != nil {
		t.Fatal(err)
	}
	result, err := peer.SyncContinuity("review")
	if err != nil || result.Received != 1 {
		t.Fatal("favorite was not received", err)
	}
	workspace, err = peer.GetWorkspace()
	if err != nil || string(workspace.UI["favorites"]) != `["other-device-track"]` {
		t.Fatal("favorite did not use the peer's track ID", err)
	}
}

func TestV3OfflinePackAndActualClip(t *testing.T) {
	s := testStudio(t)
	path := filepath.Join(s.root, "audio.flac")
	if err := os.WriteFile(path, fixtureAudio(t, s), 0600); err != nil {
		t.Fatal(err)
	}
	if n, err := s.importPaths(context.Background(), []string{path}); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	task := s.GetState().Jobs[0]
	if err := s.StartOfflinePack("Fixture pack", []string{task.ID}, 8<<20, "ar", "en"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(20 * time.Second)
	for s.GetOfflineStatus().Running && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	packs := s.ListOfflinePacks()
	if len(packs) != 1 || len(packs[0].Items) != 1 || packs[0].Items[0].File == "" {
		t.Fatal("offline copy missing", s.GetOfflineStatus())
	}
	if err := os.Rename(path, path+".moved"); err != nil {
		t.Fatal(err)
	}
	if fallback := s.audioForPlayback(task); fallback == path {
		t.Fatal("playback did not select the offline copy")
	}
	id, err := s.BeginLyricClip(task.ID, 0, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	var frame bytes.Buffer
	if err = png.Encode(&frame, image.NewRGBA(image.Rect(0, 0, 960, 540))); err != nil {
		t.Fatal(err)
	}
	if err = s.AddLyricClipFrame(id, 1, base64.StdEncoding.EncodeToString(frame.Bytes())); err != nil {
		t.Fatal(err)
	}
	if err = s.FinishLyricClip(id); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(20 * time.Second)
	for s.GetLyricClipStatus().Running && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	video, err := s.LyricClipPath(id)
	if err != nil {
		t.Fatal(err, s.GetLyricClipStatus())
	}
	info, err := s.command(context.Background(), "ffprobe", "-v", "error", "-show_entries", "stream=codec_type", "-of", "csv=p=0", video).Output()
	if err != nil || !bytes.Contains(info, []byte("video")) || !bytes.Contains(info, []byte("audio")) {
		t.Fatal("exported MP4 does not contain both streams", err)
	}
}
