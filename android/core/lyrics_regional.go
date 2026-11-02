package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"golang.org/x/net/html"
	unicodeNorm "golang.org/x/text/unicode/norm"
)

var regionalLyricOrder = []string{"synclrc", "utanet", "anghami-lyrics", "lyricstranslate"}
var regionalLyricNames = map[string]string{"synclrc": "SyncLRC", "utanet": "Uta-Net", "anghami-lyrics": "كلمات أنغامي", "lyricstranslate": "LyricsTranslate"}

func init() {
	for _, id := range regionalLyricOrder {
		lyricProviderNames[id] = regionalLyricNames[id]
		defaultLyricProviders = append(defaultLyricProviders, id)
	}
}

// Only public search/page endpoints are read; no cookies, keys or browser login.
func (s *Studio) regionalLyricBody(ctx context.Context, provider, base, path, method string, values url.Values) ([]byte, error) {
	address := s.metadataBase(provider, base) + path
	var body io.Reader
	if method == http.MethodPost {
		body = strings.NewReader(values.Encode())
	} else if len(values) > 0 {
		address += "?" + values.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, address, body)
	if err != nil {
		return nil, errors.New("عنوان مصدر الكلمات غير صالح")
	}
	req.Header.Set("User-Agent", "NawaMusic/1.1 (lyrics metadata)")
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	client := *s.client
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) > 3 || next.URL.Host != req.URL.Host || next.URL.Scheme != req.URL.Scheme {
			return errors.New("provider redirect rejected")
		}
		return nil
	}
	res, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("تعذر الاتصال بمصدر الكلمات")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s HTTP %d", regionalLyricNames[provider], res.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, (3<<20)+1))
	if err != nil || len(raw) > 3<<20 || len(bytes.TrimSpace(raw)) == 0 {
		return nil, errors.New("استجابة الكلمات فارغة أو أكبر من الحد")
	}
	return raw, nil
}
func lyricAttr(n *html.Node, key string) string {
	if n == nil {
		return ""
	}
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}
func lyricClass(n *html.Node, c string) bool {
	return strings.Contains(" "+lyricAttr(n, "class")+" ", " "+c+" ")
}
func findLyricNode(n *html.Node, predicate func(*html.Node) bool) *html.Node {
	if n == nil {
		return nil
	}
	if predicate(n) {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findLyricNode(c, predicate); found != nil {
			return found
		}
	}
	return nil
}
func lyricNodes(n *html.Node, predicate func(*html.Node) bool) []*html.Node {
	var found []*html.Node
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if predicate(n) {
			found = append(found, n)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(n)
	return found
}
func lyricHTMLText(n *html.Node, lines bool) string {
	var text strings.Builder
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n == nil {
			return
		}
		if n.Type == html.TextNode {
			text.WriteString(n.Data)
			return
		}
		if n.Data == "script" || n.Data == "style" || n.Data == "rt" || n.Data == "rp" || n.Data == "sup" || strings.HasPrefix(lyricAttr(n, "id"), "adngin-") {
			return
		}
		if n.Data == "br" {
			if lines {
				text.WriteByte('\n')
			} else {
				text.WriteByte(' ')
			}
			return
		}
		block := lines && (n.Data == "p" || lyricClass(n, "par") || lyricClass(n, "emptyline"))
		if block {
			text.WriteByte('\n')
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
		if block {
			text.WriteByte('\n')
		}
	}
	visit(n)
	if !lines {
		return strings.Join(strings.Fields(text.String()), " ")
	}
	rows := strings.Split(strings.ReplaceAll(text.String(), "\r\n", "\n"), "\n")
	for i := range rows {
		rows[i] = strings.TrimSpace(rows[i])
	}
	return strings.TrimSpace(strings.Join(rows, "\n"))
}
func regionalName(s string) string {
	return strings.Join(strings.Fields(strings.Map(func(r rune) rune {
		if r == 'ـ' || r == '\u200e' || r == '\u200f' || r >= '\u064b' && r <= '\u065f' || r == '\u0670' {
			return -1
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r) {
			return unicode.ToLower(r)
		}
		return ' '
	}, unicodeNorm.NFKC.String(s))), " ")
}
func regionalTitleMatch(want, got string) bool {
	if regionalName(want) == regionalName(got) {
		return true
	}
	// Some multilingual catalogs append a transliteration. Do not strip version
	// labels such as Live/Remix/Cover when both names use the same script.
	m := regexp.MustCompile(`^(.+?)\s+\(([^()]+)\)$`).FindStringSubmatch(got)
	if len(m) != 3 {
		return false
	}
	nonLatin := func(s string) bool {
		for _, r := range s {
			if unicode.IsLetter(r) && !unicode.Is(unicode.Latin, r) {
				return true
			}
		}
		return false
	}
	return nonLatin(m[1]) != nonLatin(m[2]) && (regionalName(want) == regionalName(m[1]) || regionalName(want) == regionalName(m[2]))
}
func regionalRecordingMatch(song Song, title, artist string, duration float64) bool {
	if math.IsNaN(duration) || math.IsInf(duration, 0) || duration < 0 {
		return false
	}
	if !regionalTitleMatch(song.Title, title) || regionalName(song.Artist) != regionalName(artist) {
		return false
	}
	return lyricsRecordingMatch(Song{Title: title, Artist: artist, Duration: song.Duration}, title, artist, duration)
}
func regionalPath(path, prefix string) bool {
	u, e := url.Parse(path)
	return e == nil && u.Host == "" && u.Scheme == "" && strings.HasPrefix(u.Path, prefix) && !strings.Contains(u.Path, "..") && u.RawQuery == ""
}

func (s *Studio) fetchRegionalLyricProvider(ctx context.Context, song Song, provider string) (LyricsRecord, error) {
	empty := LyricsRecord{}
	if regionalName(song.Title) == "" || regionalName(song.Artist) == "" {
		return empty, errors.New("اسم الأغنية والفنان مطلوبان")
	}
	switch provider {
	case "synclrc":
		params := url.Values{"track": {song.Title}, "artist": {song.Artist}}
		if song.Album != "" {
			params.Set("album", song.Album)
		}
		if song.Duration > 0 {
			params.Set("duration", fmt.Sprintf("%.3f", song.Duration))
		}
		raw, e := s.regionalLyricBody(ctx, provider, "https://api.synclrc.dev", "/lyrics", http.MethodGet, params)
		if e != nil {
			return empty, e
		}
		var result struct {
			Track, Artist, Album, Plain, Synced string
			Duration                            flexFloat
			Instrumental                        bool
		}
		if json.Unmarshal(raw, &result) != nil || !regionalRecordingMatch(song, result.Track, result.Artist, float64(result.Duration)) {
			return empty, errors.New("SyncLRC: بيانات التسجيل لا تطابق الأغنية")
		}
		if result.Plain == "" && !validLRC(result.Synced, song.Duration) && !result.Instrumental {
			return empty, errors.New("SyncLRC: لا توجد كلمات متاحة")
		}
		return LyricsRecord{Title: result.Track, Artist: result.Artist, Album: result.Album, Duration: float64(result.Duration), Plain: result.Plain, Synced: result.Synced, Instrumental: result.Instrumental}, nil
	case "utanet":
		raw, e := s.regionalLyricBody(ctx, provider, "https://www.uta-net.com", "/search/", http.MethodGet, url.Values{"Aselect": {"2"}, "Bselect": {"3"}, "Keyword": {song.Title}, "sort": {"4"}})
		if e != nil {
			return empty, e
		}
		doc, e := html.Parse(bytes.NewReader(raw))
		if e != nil {
			return empty, e
		}
		for _, row := range lyricNodes(doc, func(n *html.Node) bool { return n.Data == "tr" }) {
			title := findLyricNode(row, func(n *html.Node) bool { return lyricClass(n, "songlist-title") })
			artist := findLyricNode(row, func(n *html.Node) bool { return n.Data == "a" && strings.HasPrefix(lyricAttr(n, "href"), "/artist/") })
			link := findLyricNode(row, func(n *html.Node) bool { return n.Data == "a" && regionalPath(lyricAttr(n, "href"), "/song/") })
			if link == nil || !regionalRecordingMatch(song, lyricHTMLText(title, false), lyricHTMLText(artist, false), 0) {
				continue
			}
			raw, e = s.regionalLyricBody(ctx, provider, "https://www.uta-net.com", lyricAttr(link, "href"), http.MethodGet, nil)
			if e != nil {
				return empty, e
			}
			doc, e = html.Parse(bytes.NewReader(raw))
			if e != nil {
				return empty, e
			}
			body := findLyricNode(doc, func(n *html.Node) bool { return lyricAttr(n, "id") == "kashi_area" })
			return plainLyricRecord(song, lyricHTMLText(body, true))
		}
	case "lyricstranslate":
		params := url.Values{"formtree[ltsearchtextsong]": {song.Title}, "formtree[ltsearchtextartist]": {song.Artist}, "form_id": {"lyricstranslatesearch_searchsong_form"}, "op": {"Search"}}
		raw, e := s.regionalLyricBody(ctx, provider, "https://lyricstranslate.com", "/en/songs", http.MethodPost, params)
		if e != nil {
			return empty, e
		}
		doc, e := html.Parse(bytes.NewReader(raw))
		if e != nil {
			return empty, e
		}
		for _, row := range lyricNodes(doc, func(n *html.Node) bool { return lyricClass(n, "block-1-table") }) {
			title := findLyricNode(row, func(n *html.Node) bool { return lyricClass(n, "title-text") })
			artist := findLyricNode(row, func(n *html.Node) bool { return lyricClass(n, "block-1-table__author") })
			link := findLyricNode(row, func(n *html.Node) bool { return n.Data == "a" && lyricClass(n, "title-link") })
			path := lyricAttr(link, "href")
			if !regionalPath(path, "/en/") || !strings.HasSuffix(path, "-lyrics.html") || !regionalRecordingMatch(song, lyricHTMLText(title, false), lyricHTMLText(artist, false), 0) {
				continue
			}
			raw, e = s.regionalLyricBody(ctx, provider, "https://lyricstranslate.com", path, http.MethodGet, nil)
			if e != nil {
				return empty, e
			}
			doc, e = html.Parse(bytes.NewReader(raw))
			if e != nil {
				return empty, e
			}
			// song-body is the original text, never a translation/transliteration.
			body := findLyricNode(doc, func(n *html.Node) bool { return lyricAttr(n, "id") == "song-body" })
			return plainLyricRecord(song, lyricHTMLText(body, true))
		}
	case "anghami-lyrics":
		params := url.Values{"type": {"GETSearchResults"}, "query": {song.Title + " " + song.Artist}, "filter_type": {"kalimat"}, "count": {"18"}, "page": {"1"}, "intra": {"true"}, "output": {"jsonhp"}, "lang": {"ar"}, "language": {"ar"}, "web_medium": {"lyrics"}}
		raw, e := s.regionalLyricBody(ctx, provider, "https://coussa.anghami.com", "/gateway.php", http.MethodGet, params)
		if e != nil {
			return empty, e
		}
		var result struct {
			Status   string
			Sections []struct {
				Data []struct {
					ID, Title, Artist string
					Duration          flexFloat
				}
			}
		}
		if json.Unmarshal(raw, &result) != nil || result.Status != "ok" {
			return empty, errors.New("كلمات أنغامي: استجابة البحث غير صالحة")
		}
		for _, section := range result.Sections {
			for _, track := range section.Data {
				if !regexp.MustCompile(`^[0-9]{1,16}$`).MatchString(track.ID) || !regionalRecordingMatch(song, track.Title, track.Artist, float64(track.Duration)) {
					continue
				}
				raw, e = s.regionalLyricBody(ctx, provider, "https://kalimat.anghami.com", "/lyrics/"+track.ID, http.MethodGet, nil)
				if e != nil {
					return empty, e
				}
				doc, e := html.Parse(bytes.NewReader(raw))
				if e != nil {
					return empty, e
				}
				title := findLyricNode(doc, func(n *html.Node) bool { return n.Data == "h1" })
				artist := findLyricNode(doc, func(n *html.Node) bool { return n.Data == "h2" })
				if !regionalRecordingMatch(song, lyricHTMLText(title, false), lyricHTMLText(artist, false), float64(track.Duration)) {
					return empty, errors.New("كلمات أنغامي: الصفحة لا تطابق التسجيل")
				}
				body := findLyricNode(doc, func(n *html.Node) bool {
					return n.Data == "pre" && strings.Contains(lyricAttr(n, "class"), "lyrics_body_container")
				})
				return plainLyricRecord(song, lyricHTMLText(body, true))
			}
		}
	default:
		return empty, errors.New("مصدر كلمات غير معروف")
	}
	return empty, errors.New("لم توجد كلمات أصلية تطابق العنوان والفنان")
}
