package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

// Opt-in public-source smoke check; temporary data only, no user library/audio.
func TestLiveRegionalLyrics(t *testing.T) {
	if os.Getenv("NAWA_REGIONAL_LIVE") != "1" {
		t.Skip("optional live public-source check")
	}
	for _, tc := range []struct {
		provider, title, artist string
		duration                float64
	}{
		{"utanet", "アイドル", "YOASOBI", 0},
		{"anghami-lyrics", "معقول", "فضل شاكر", 339.75},
		{"lyricstranslate", "Камин", "EMIN", 0},
		{"synclrc", "YOASOBI「群青」", "YOASOBI", 262},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			s := testStudio(t)
			r, e := s.fetchRegionalLyricProvider(context.Background(), Song{Title: tc.title, Artist: tc.artist, Duration: tc.duration}, tc.provider)
			if e != nil {
				t.Fatal(e)
			}
			if r.Plain == "" && r.Synced == "" {
				t.Fatal("empty provider result")
			}
			t.Logf("original text received: %d bytes; synced=%t", len(r.Plain), r.Synced != "")
		})
	}
}

func TestRegionalLyricsUnicodeAndOfflineCache(t *testing.T) {
	for _, tc := range []struct{ provider, title, artist, text string }{
		{"utanet", "夜の灯り", "試験音楽", "夜の灯り\n音がつながる"},
		{"anghami-lyrics", "نور المساء", "موسيقى الاختبار", "نور المساء\nنجتمع على النغم"},
		{"lyricstranslate", "Контур", "Тестовая музыка", "Тихий звук\nНовый день"},
		{"lyricstranslate", "빛의 시간", "시험 음악", "빛의 시간\n음악이 흐른다"},
		{"lyricstranslate", "Işığın sesi", "Deneme müziği", "Işığın sesi\nYeni bir gün"},
		{"lyricstranslate", "Lumière du soir", "Musique essai", "Lumière du soir\nLe son nous réunit"},
		{"lyricstranslate", "रोशनी का स्वर", "परीक्षण संगीत", "रोशनी का स्वर\nनया दिन"},
		{"synclrc", "Quiet lights", "Nawa fixtures", "Quiet lights\nSounds bring us together"},
	} {
		t.Run(tc.provider+"-"+tc.title, func(t *testing.T) {
			s := testStudio(t)
			song := Song{ID: "regional-original", Title: tc.title, Artist: tc.artist, Duration: 24}
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				switch r.URL.Path {
				case "/search/":
					if r.URL.Query().Get("Keyword") != tc.title {
						t.Error("Japanese search corrupted Unicode")
					}
					fmt.Fprintf(w, `<table><tr><td><a href="/song/123/"><span class="songlist-title">%s</span></a></td><td><a href="/artist/456/">%s</a></td></tr></table>`, tc.title, tc.artist)
				case "/song/123/":
					fmt.Fprintf(w, `<aside>Not lyrics</aside><div id="kashi_area">%s<script>ignore()</script></div>`, strings.ReplaceAll(tc.text, "\n", "<br>"))
				case "/en/songs":
					if r.Method != http.MethodPost || r.ParseForm() != nil || r.PostForm.Get("formtree[ltsearchtextsong]") != tc.title || r.PostForm.Get("formtree[ltsearchtextartist]") != tc.artist {
						t.Error("multilingual search contract")
					}
					fmt.Fprintf(w, `<div class="block-1-table"><a class="title-link" href="/en/fixture-lyrics.html"><span class="title-text">%s</span></a><span class="block-1-table__author">%s</span></div>`, tc.title, tc.artist)
				case "/en/fixture-lyrics.html":
					fmt.Fprintf(w, `<div id="translation-body">WRONG TRANSLATION</div><div id="song-body"><div class="ltf"><div class="par">%s</div></div></div>`, strings.ReplaceAll(tc.text, "\n", "<br>"))
				case "/gateway.php":
					if r.URL.Query().Get("type") != "GETSearchResults" || r.URL.Query().Get("query") != tc.title+" "+tc.artist {
						t.Error("Arabic search contract")
					}
					json.NewEncoder(w).Encode(map[string]any{"status": "ok", "sections": []any{map[string]any{"data": []any{map[string]any{"id": "123", "title": tc.title, "artist": tc.artist, "duration": "24"}}}}})
				case "/lyrics/123":
					fmt.Fprintf(w, `<h1>%s</h1><h2>%s</h2><pre class="lyrics-body-module__fixture__lyrics_body_container">%s</pre>`, tc.title, tc.artist, tc.text)
				case "/lyrics":
					if r.URL.Query().Get("track") != tc.title || r.URL.Query().Get("artist") != tc.artist {
						t.Error("SyncLRC contract")
					}
					json.NewEncoder(w).Encode(map[string]any{"track": tc.title, "artist": tc.artist, "duration": 24, "plain": tc.text, "synced": "[00:00.00]Quiet lights\n[00:03.00]Sounds bring us together"})
				default:
					t.Error("unexpected route", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			s.sourceTestBases = map[string]string{tc.provider: server.URL}
			r, e := s.remoteLyricsFrom(context.Background(), song, tc.provider)
			if e != nil || r.Plain != tc.text {
				t.Fatalf("original Unicode text lost: %v, %+v", e, r)
			}
			if tc.provider != "synclrc" && (r.Synced != "" || len(r.Words) != 0) {
				t.Fatal("plain lyrics were assigned fake audio timing")
			}
			n := calls.Load()
			server.Close()
			r, e = s.remoteLyricsFrom(context.Background(), song, "auto")
			if e != nil || !r.Cached || r.Plain != tc.text || calls.Load() != n {
				t.Fatal("offline cache lost", e)
			}
		})
	}
}
func TestRegionalLyricsRejectWrongIdentityAndUnsafeLinks(t *testing.T) {
	for _, provider := range regionalLyricOrder {
		t.Run(provider, func(t *testing.T) {
			s := testStudio(t)
			song := Song{Title: "Fixture", Artist: "Nawa", Duration: 24}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if provider == "synclrc" {
					fmt.Fprint(w, `{"track":"Fixture (Live)","artist":"Nawa","duration":24,"plain":"Wrong recording"}`)
					return
				}
				if provider == "anghami-lyrics" {
					fmt.Fprint(w, `{"status":"ok","sections":[{"data":[{"id":"123","title":"Fixture","artist":"Other artist","duration":24}]}]}`)
					return
				}
				fmt.Fprint(w, `<tr><td><a href="https://example.invalid/song/1/"><span class="songlist-title">Fixture</span></a></td><td><a href="/artist/1/">Nawa</a></td></tr><div class="block-1-table"><a class="title-link" href="//example.invalid/en/fixture-lyrics.html"><span class="title-text">Fixture</span></a><span class="block-1-table__author">Nawa</span></div>`)
			}))
			defer server.Close()
			s.sourceTestBases = map[string]string{provider: server.URL}
			if _, e := s.fetchRegionalLyricProvider(context.Background(), song, provider); e == nil {
				t.Fatal("unmatched/unsafe result accepted")
			}
		})
	}
	if !regionalTitleMatch("Контур", "Контур (Kontur)") || regionalTitleMatch("Fixture", "Fixture (Live)") {
		t.Fatal("transliteration/version identity check")
	}
	if regionalTitleMatch("कल", "कला") {
		t.Fatal("Indic vowel signs were discarded during matching")
	}
}
func TestRegionalLyricsFallbackAndSourceUpgrade(t *testing.T) {
	s := testStudio(t)
	s.state.Preferences.MetadataSourcesVersion = 1
	s.state.Preferences.LyricProviders = []string{"lrclib", "utanet"}
	s.save()
	s = NewStudio(s.root)
	seen := map[string]int{}
	for _, id := range s.GetState().Preferences.LyricProviders {
		seen[id]++
	}
	for _, id := range regionalLyricOrder {
		if seen[id] != 1 {
			t.Fatal("new source missing or duplicated", id, seen)
		}
	}
	if s.state.Preferences.LyricProviders[0] != "lrclib" || s.state.Preferences.MetadataSourcesVersion != 2 {
		t.Fatal("upgrade changed order or lost its version")
	}
	s.save()
	again := NewStudio(s.root)
	if len(again.state.Preferences.LyricProviders) != len(s.state.Preferences.LyricProviders) {
		t.Fatal("upgrade repeated sources on restart")
	}
	s.state.Preferences.LyricProviders = []string{"utanet", "synclrc"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/search/" {
			w.WriteHeader(503)
			return
		}
		fmt.Fprint(w, `{"track":"Fixture","artist":"Nawa","duration":24,"plain":"Original fixture line\nAnother fixture line"}`)
	}))
	defer server.Close()
	s.sourceTestBases = map[string]string{"utanet": server.URL, "synclrc": server.URL}
	r, e := s.remoteLyricsFrom(context.Background(), Song{ID: "fallback", Title: "Fixture", Artist: "Nawa", Duration: 24}, "auto")
	if e != nil || r.Provider != "synclrc" {
		t.Fatal("automatic fallback failed", e, r.Provider)
	}
}
