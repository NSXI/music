package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

const extraLyricsPlain = "First test line\nSecond test line"
const extraLyricsTTML = `<tt xmlns="http://www.w3.org/ns/ttml"><head><metadata><text for="L1">IGNORE TRANSLITERATION</text></metadata></head><body><div><p begin="0:00.100" end="0:00.400"><span begin="0:00.100" end="0:00.240">First </span><span begin="0:00.240" end="0:00.400">test line</span></p><p begin="00:00:00.600" end="00:00:00.950"><span begin="00:00:00.600" end="00:00:00.950">Second test line</span></p></div></body></tt>`

func newExtraLyricsServer(t *testing.T, provider string, wrong bool) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	requests := &atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		title, artist := "Fixture recording", "Madar Tests"
		if wrong {
			title += " (Live)"
		}
		switch provider {
		case "lyricsovh":
			if r.URL.EscapedPath() != "/v1/Madar%20Tests/Fixture%20recording" {
				t.Errorf("unexpected escaped route: %s", r.URL.EscapedPath())
			}
			json.NewEncoder(w).Encode(map[string]string{"lyrics": extraLyricsPlain})
		case "lrcapi":
			if r.URL.Path != "/api/v1/lyrics/advance" || r.URL.Query().Get("artist") != "Madar Tests" {
				t.Error("wrong LrcAPI contract")
			}
			json.NewEncoder(w).Encode([]map[string]any{{"title": title, "artist": artist, "lyrics": fixtureLRC, "duration": 1}})
		case "betterlyrics":
			if r.URL.Path != "/getLyrics" || r.URL.Query().Get("s") != "Fixture recording" || r.URL.Query().Get("a") != "Madar Tests" || r.URL.Query().Get("d") != "1.000" {
				t.Error("wrong Better Lyrics query")
			}
			json.NewEncoder(w).Encode(map[string]any{"ttml": extraLyricsTTML, "score": .98, "title": title, "artist": artist, "duration": 1})
		case "vagalume":
			if r.URL.Path != "/search.php" || r.URL.Query().Get("apikey") != "fixture-private-key" || r.URL.Query().Get("art") != "Madar Tests" {
				t.Error("wrong Vagalume query")
			}
			json.NewEncoder(w).Encode(map[string]any{"type": "exact", "art": map[string]string{"name": artist}, "mus": []map[string]string{{"name": title, "text": extraLyricsPlain}}})
		case "audd":
			if r.Method != http.MethodPost || r.URL.Path != "/findLyrics/" || r.ParseForm() != nil || r.PostForm.Get("api_token") != "fixture-private-key" || r.PostForm.Get("q") != "Madar Tests Fixture recording" || r.URL.Query().Get("api_token") != "" {
				t.Error("wrong AudD POST contract")
			}
			json.NewEncoder(w).Encode(map[string]any{"status": "success", "result": []map[string]string{{"title": title, "artist": artist, "lyrics": extraLyricsPlain}}})
		}
	}))
	return server, requests
}

func TestAdditionalFiveLyricsContractsAndOfflineCache(t *testing.T) {
	for _, provider := range additionalLyricOrder {
		t.Run(provider, func(t *testing.T) {
			s := testStudio(t)
			task := lyricTask(t, s)
			s.metadataKeys = map[string]string{"vagalume": "fixture-private-key", "audd": "fixture-private-key"}
			server, requests := newExtraLyricsServer(t, provider, false)
			s.sourceTestBases = map[string]string{provider: server.URL}
			r, err := s.remoteLyricsFrom(context.Background(), task.Song, provider)
			if err != nil || r.Provider != provider {
				server.Close()
				t.Fatalf("provider: %v, %+v", err, r)
			}
			switch provider {
			case "betterlyrics":
				if r.Synced == "" || len(r.Words) != 2 || len(r.Words[0].Words) != 2 || r.Words[0].Words[1].Start != .24 || r.Words[1].Words[0].End != .95 {
					t.Fatalf("explicit TTML times were lost: %+v", r.Words)
				}
			case "lrcapi":
				if r.Synced != fixtureLRC || len(r.Words) != 0 {
					t.Fatal("LRC source must retain line timing without invented word times")
				}
			default:
				if r.Synced != "" || r.Plain != extraLyricsPlain || len(r.Words) != 0 {
					t.Fatal("plain source was incorrectly timed")
				}
			}
			cache, err := s.readLyricCache(task.Song)
			if err != nil || cache.Record.Provider != provider {
				t.Fatal("cache not persisted", err)
			}
			if provider == "betterlyrics" && len(cache.Words) != 2 {
				t.Fatal("word timestamps missing from on-disk cache")
			}
			calls := requests.Load()
			server.Close()
			r, err = s.remoteLyricsFrom(context.Background(), task.Song, "auto")
			if err != nil || !r.Cached || r.Provider != provider || requests.Load() != calls {
				t.Fatal("offline cache should be read before networking", err)
			}
			if provider == "betterlyrics" && len(r.Words) != 2 {
				t.Fatal("word timestamps not restored offline")
			}
		})
	}
}
func TestAdditionalLyricsRejectWrongRecording(t *testing.T) {
	for _, provider := range []string{"lrcapi", "betterlyrics", "vagalume", "audd"} {
		t.Run(provider, func(t *testing.T) {
			s := testStudio(t)
			task := lyricTask(t, s)
			s.metadataKeys = map[string]string{provider: "fixture-private-key"}
			server, _ := newExtraLyricsServer(t, provider, true)
			defer server.Close()
			s.sourceTestBases = map[string]string{provider: server.URL}
			if _, err := s.fetchAdditionalLyricProvider(context.Background(), task.Song, provider); err == nil {
				t.Fatal("live version accepted for studio title")
			}
		})
	}
}

type lyricsFailTransport struct{}

func (lyricsFailTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("secret echoed: fixture-private-key")
}
func TestAdditionalLyricsMissingKeysLimitsAndSecretErrors(t *testing.T) {
	for _, provider := range []string{"vagalume", "audd"} {
		s := testStudio(t)
		task := lyricTask(t, s)
		s.client.Transport = lyricsFailTransport{}
		if _, err := s.fetchAdditionalLyricProvider(context.Background(), task.Song, provider); err == nil || !strings.Contains(err.Error(), "يحتاج") {
			t.Fatal("missing credential must be explicit", err)
		}
		s.metadataKeys = map[string]string{provider: "fixture-private-key"}
		if _, err := s.fetchAdditionalLyricProvider(context.Background(), task.Song, provider); err == nil || strings.Contains(err.Error(), "fixture-private-key") {
			t.Fatal("connection error leaked key", err)
		}
	}
	s := testStudio(t)
	task := lyricTask(t, s)
	for _, test := range []struct {
		name, body string
		code       int
	}{{"html", "<html>login required</html>", 200}, {"oversize", strings.Repeat("x", (4<<20)+1), 200}, {"http", "key echoed should not leak", 429}, {"json", "{bad json", 200}, {"empty", "", 200}} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(test.code); io.WriteString(w, test.body) }))
			defer server.Close()
			s.sourceTestBases = map[string]string{"lyricsovh": server.URL}
			if _, err := s.fetchAdditionalLyricProvider(context.Background(), task.Song, "lyricsovh"); err == nil {
				t.Fatal("invalid source response accepted")
			}
		})
	}
}
func TestExplicitTTMLValidationAndWhitespace(t *testing.T) {
	song := Song{Title: "Own test", Artist: "Madar", Duration: 10}
	r, err := decodeBetterTTML(`<tt xmlns="http://www.w3.org/ns/ttml"><body><div><p begin="0.1s" end="900ms"><span begin="0.1s" end=".4s">كلمات</span> <span begin=".4s" end=".9s">عربية</span></p><p begin="1.1s" end="2s"><span><span begin="1.1s" end="1.6s">متصلة</span> <span begin="1.6s" end="2s">معًا</span></span></p></div></body></tt>`, song)
	if err != nil || r.Plain != "كلمات عربية\nمتصلة معًا" || len(r.Words) != 2 || r.Words[0].Words[1].Start != .4 {
		t.Fatalf("text order/explicit timing: %+v %v", r, err)
	}
	for _, raw := range []string{`<!DOCTYPE tt [<!ENTITY x SYSTEM "file:///secret">]><tt><body><p begin="0" end="1">&x;</p></body></tt>`, `<tt><body><p begin="0" end="NaN">bad</p></body></tt>`, `<tt><body><p begin="0" end="1">line</p></body></tt><html/>`, `<tt><body><p begin="00:00:60" end="00:02:00">bad</p></body></tt>`, `<tt><body><p begin="0" end="0">bad</p></body></tt>`} {
		if _, err := decodeBetterTTML(raw, song); err == nil {
			t.Errorf("invalid TTML accepted: %.60s", raw)
		}
	}
	for _, v := range []struct {
		text    string
		seconds float64
	}{{"01:02:03.125", 3723.125}, {"1:16.656", 76.656}, {"1500ms", 1.5}, {".25s", .25}} {
		got, ok := lyricTime(v.text)
		if !ok || got != v.seconds {
			t.Fatal(fmt.Sprintf("time parse %s: %v %v", v.text, got, ok))
		}
	}
	// A span without a genuine end remains line timing; no end is invented.
	r, err = decodeBetterTTML(`<tt><body><p begin="0" end="1"><span begin="0">One</span></p><p begin="2" end="3">Two</p></body></tt>`, song)
	if err != nil || len(r.Words) != 2 || len(r.Words[0].Words) != 0 {
		t.Fatal("missing explicit word end fabricated", err)
	}
}
