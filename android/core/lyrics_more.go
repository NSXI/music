package main

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

var additionalLyricNames = map[string]string{
	"lyricsovh": "Lyrics.ovh", "betterlyrics": "Better Lyrics", "lrcapi": "LrcAPI", "vagalume": "Vagalume", "audd": "AudD Lyrics",
}
var additionalLyricOrder = []string{"betterlyrics", "lrcapi", "lyricsovh", "vagalume", "audd"}

func init() {
	for id, name := range additionalLyricNames {
		lyricProviderNames[id] = name
	}
	for _, provider := range additionalLyricOrder {
		if provider != "audd" {
			defaultLyricProviders = append(defaultLyricProviders, provider)
		}
	}
}

// API adapters only: no scrape scripts, user account reuse, or generated timing.
func (s *Studio) extraLyricBody(ctx context.Context, provider, method, base, path string, params url.Values, headers map[string]string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	address := s.metadataBase(provider, base) + path
	var body io.Reader
	if method == http.MethodPost {
		body = strings.NewReader(params.Encode())
	} else if len(params) != 0 {
		address += "?" + params.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, address, body)
	if err != nil {
		return nil, errors.New("عنوان مصدر الكلمات غير صالح")
	}
	req.Header.Set("User-Agent", "MadarMusic/1.0 (lyrics metadata)")
	req.Header.Set("Accept", "application/json")
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	client := *s.client
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) >= 3 || !strings.EqualFold(next.URL.Host, req.URL.Host) || next.URL.Scheme != req.URL.Scheme {
			return errors.New("provider redirect rejected")
		}
		return nil
	}
	response, err := client.Do(req)
	if err != nil {
		// http.Client errors include the complete query URL, which may hold keys.
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%s: تعذر الاتصال", additionalLyricNames[provider])
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		if provider == "betterlyrics" && response.StatusCode == http.StatusUnauthorized {
			return nil, errors.New("Better Lyrics: الكلمات غير مخزنة لديهم؛ الجلب الجديد يحتاج مفتاحًا صالحًا")
		}
		return nil, fmt.Errorf("%s HTTP %d", additionalLyricNames[provider], response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	if err != nil {
		return nil, errors.New("تعذر قراءة استجابة مصدر الكلمات")
	}
	if len(data) > 4<<20 || len(bytes.TrimSpace(data)) == 0 {
		return nil, errors.New("استجابة الكلمات فارغة أو أكبر من الحد المسموح")
	}
	if bytes.Contains(bytes.ToLower(data[:min(len(data), 1024)]), []byte("<html")) {
		return nil, errors.New("المصدر أعاد صفحة ويب بدلاً من بيانات الكلمات")
	}
	return data, nil
}

func plainLyricRecord(song Song, text string) (LyricsRecord, error) {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
	if text == "" || len(text) > 2<<20 {
		return LyricsRecord{}, errors.New("لا توجد كلمات صالحة")
	}
	return LyricsRecord{Title: song.Title, Artist: song.Artist, Album: song.Album, Duration: song.Duration, Plain: text}, nil
}

func (s *Studio) fetchAdditionalLyricProvider(ctx context.Context, song Song, provider string) (LyricsRecord, error) {
	empty := LyricsRecord{}
	if norm(song.Title) == "" || norm(song.Artist) == "" {
		return empty, errors.New("اسم الأغنية والفنان مطلوبان لمطابقة الكلمات")
	}
	switch provider {
	case "lyricsovh":
		// The exact artist/title endpoint has no independent identity fields.
		body, err := s.extraLyricBody(ctx, provider, http.MethodGet, "https://api.lyrics.ovh", "/v1/"+url.PathEscape(song.Artist)+"/"+url.PathEscape(song.Title), nil, nil)
		if err != nil {
			return empty, err
		}
		var result struct {
			Lyrics string `json:"lyrics"`
		}
		if json.Unmarshal(body, &result) != nil {
			return empty, errors.New("استجابة Lyrics.ovh غير صالحة")
		}
		return plainLyricRecord(song, result.Lyrics)
	case "lrcapi":
		params := url.Values{"title": {song.Title}, "artist": {song.Artist}, "album": {song.Album}}
		body, err := s.extraLyricBody(ctx, provider, http.MethodGet, "https://api.lrc.cx", "/api/v1/lyrics/advance", params, nil)
		if err != nil {
			return empty, err
		}
		var result []struct {
			Title, Artist, Lyrics string
			Duration              flexFloat `json:"duration"`
		}
		if json.Unmarshal(body, &result) != nil {
			return empty, errors.New("استجابة LrcAPI غير صالحة")
		}
		for _, record := range result {
			if !lyricsRecordingMatch(song, record.Title, record.Artist, float64(record.Duration)) {
				continue
			}
			if validLRC(record.Lyrics, song.Duration) {
				return LyricsRecord{Title: record.Title, Artist: record.Artist, Duration: song.Duration, Synced: record.Lyrics}, nil
			}
			// LrcAPI is a synchronized-lyrics API: malformed timing is rejected,
			// rather than displaying its markup as purported plain lyrics.
		}
	case "betterlyrics":
		params := url.Values{"s": {song.Title}, "a": {song.Artist}, "al": {song.Album}}
		if song.Duration > 0 {
			params.Set("d", strconv.FormatFloat(song.Duration, 'f', 3, 64))
		}
		headers := map[string]string{}
		if key := s.metadataKey(provider); key != "" {
			headers["X-API-Key"] = key
		}
		body, err := s.extraLyricBody(ctx, provider, http.MethodGet, "https://api.betterlyrics.org", "/getLyrics", params, headers)
		if err != nil {
			return empty, err
		}
		var result struct {
			TTML          string   `json:"ttml"`
			Score         *float64 `json:"score"`
			Title, Artist string
			Duration      flexFloat `json:"duration"`
		}
		if json.Unmarshal(body, &result) != nil || result.TTML == "" {
			return empty, errors.New("استجابة Better Lyrics غير صالحة")
		}
		if result.Score != nil && (*result.Score < .9 || *result.Score > 1 || math.IsNaN(*result.Score)) {
			return empty, errors.New("Better Lyrics: درجة المطابقة غير كافية")
		}
		if result.Title != "" && norm(result.Title) != norm(song.Title) || result.Artist != "" && norm(result.Artist) != norm(song.Artist) || float64(result.Duration) > 0 && !lyricsRecordingMatch(song, song.Title, song.Artist, float64(result.Duration)) {
			return empty, errors.New("Better Lyrics: بيانات الرد لا تطابق التسجيل")
		}
		return decodeBetterTTML(result.TTML, song)
	case "vagalume":
		key := s.metadataKey(provider)
		if key == "" {
			return empty, errors.New("Vagalume يحتاج مفتاح API خاصًا بك")
		}
		params := url.Values{"art": {song.Artist}, "mus": {song.Title}, "apikey": {key}, "extra": {"relmus"}}
		body, err := s.extraLyricBody(ctx, provider, http.MethodGet, "https://api.vagalume.com.br", "/search.php", params, nil)
		if err != nil {
			return empty, err
		}
		var result struct {
			Type string `json:"type"`
			Art  struct {
				Name string `json:"name"`
			} `json:"art"`
			Mus []struct {
				Name, Text string
				Duration   flexFloat `json:"duration"`
			} `json:"mus"`
		}
		if json.Unmarshal(body, &result) != nil {
			return empty, errors.New("استجابة Vagalume غير صالحة")
		}
		for _, record := range result.Mus {
			if lyricsRecordingMatch(song, record.Name, result.Art.Name, float64(record.Duration)) {
				return plainLyricRecord(song, record.Text)
			}
		}
	case "audd":
		key := s.metadataKey(provider)
		if key == "" {
			return empty, errors.New("AudD Lyrics يحتاج API token خاصًا بك")
		}
		body, err := s.extraLyricBody(ctx, provider, http.MethodPost, "https://api.audd.io", "/findLyrics/", url.Values{"q": {song.Artist + " " + song.Title}, "api_token": {key}}, nil)
		if err != nil {
			return empty, err
		}
		var result struct {
			Status string `json:"status"`
			Result []struct {
				Artist, Title, Lyrics string
				Duration              flexFloat `json:"duration"`
			} `json:"result"`
		}
		if json.Unmarshal(body, &result) != nil || result.Status != "success" {
			return empty, errors.New("AudD Lyrics: الطلب غير متاح أو المفتاح غير صالح")
		}
		for _, record := range result.Result {
			if lyricsRecordingMatch(song, record.Title, record.Artist, float64(record.Duration)) {
				return plainLyricRecord(song, record.Lyrics)
			}
		}
	default:
		if _, ok := regionalLyricNames[provider]; ok {
			return s.fetchRegionalLyricProvider(ctx, song, provider)
		}
		return empty, errors.New("مصدر كلمات غير معروف")
	}
	return empty, errors.New("لم توجد كلمات تطابق عنوان الأغنية والفنان والتسجيل")
}

// A small inert TTML reader preserves text order and only retains explicit
// begin/end timestamps. encoding/xml does not fetch external entities.
type lyricXMLPart struct {
	Text  string
	Child *lyricXMLNode
}
type lyricXMLNode struct {
	Name  string
	Attrs []xml.Attr
	Parts []lyricXMLPart
}

func readLyricXML(decoder *xml.Decoder, start xml.StartElement, depth int, nodes *int) (*lyricXMLNode, error) {
	*nodes++
	if depth > 32 || *nodes > 50000 {
		return nil, errors.New("TTML exceeds parsing limits")
	}
	node := &lyricXMLNode{Name: start.Name.Local, Attrs: start.Attr}
	for {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		switch value := token.(type) {
		case xml.StartElement:
			child, err := readLyricXML(decoder, value, depth+1, nodes)
			if err != nil {
				return nil, err
			}
			node.Parts = append(node.Parts, lyricXMLPart{Child: child})
		case xml.CharData:
			node.Parts = append(node.Parts, lyricXMLPart{Text: string(value)})
		case xml.Directive:
			return nil, errors.New("TTML directives are not accepted")
		case xml.EndElement:
			return node, nil
		}
	}
}
func (n *lyricXMLNode) attr(name string) string {
	for _, attr := range n.Attrs {
		if attr.Name.Local == name {
			return attr.Value
		}
	}
	return ""
}
func (n *lyricXMLNode) text() string {
	var body strings.Builder
	for _, part := range n.Parts {
		if part.Child != nil {
			if part.Child.Name == "br" {
				body.WriteString(" ")
			} else {
				body.WriteString(part.Child.text())
			}
		} else {
			body.WriteString(part.Text)
		}
	}
	return body.String()
}
func lyricTime(value string) (float64, bool) {
	if value == "" {
		return 0, false
	}
	if strings.HasSuffix(value, "ms") {
		n, err := strconv.ParseFloat(strings.TrimSuffix(value, "ms"), 64)
		return n / 1000, err == nil && n >= 0 && n <= 86400000 && !math.IsInf(n, 0) && !math.IsNaN(n)
	}
	value = strings.TrimSuffix(value, "s")
	parts := strings.Split(value, ":")
	if len(parts) > 3 {
		return 0, false
	}
	var total float64
	for i, part := range parts {
		n, err := strconv.ParseFloat(part, 64)
		if err != nil || n < 0 || math.IsInf(n, 0) || math.IsNaN(n) || i > 0 && n >= 60 {
			return 0, false
		}
		total = total*60 + n
	}
	return total, total <= 86400
}
func explicitTTMLWords(node *lyricXMLNode) []TimedWord {
	var result []TimedWord
	for _, part := range node.Parts {
		if part.Child == nil {
			continue
		}
		child := part.Child
		start, startOK := lyricTime(child.attr("begin"))
		end, endOK := lyricTime(child.attr("end"))
		if child.Name == "span" && startOK && endOK && end > start && strings.TrimSpace(child.text()) != "" {
			result = append(result, TimedWord{Text: strings.TrimSpace(child.text()), Start: start, End: end})
		} else {
			result = append(result, explicitTTMLWords(child)...)
		}
	}
	return result
}
func decodeBetterTTML(raw string, song Song) (LyricsRecord, error) {
	empty := LyricsRecord{}
	if len(raw) > 3<<20 {
		return empty, errors.New("TTML أكبر من الحد المسموح")
	}
	decoder := xml.NewDecoder(strings.NewReader(raw))
	var root *lyricXMLNode
	nodes := 0
	for {
		token, err := decoder.Token()
		if err != nil {
			return empty, errors.New("TTML غير صالح")
		}
		if _, ok := token.(xml.Directive); ok {
			return empty, errors.New("TTML يحتوي تعريفات غير مسموحة")
		}
		if start, ok := token.(xml.StartElement); ok {
			root, err = readLyricXML(decoder, start, 0, &nodes)
			if err != nil || root.Name != "tt" {
				return empty, errors.New("TTML غير صالح أو يتجاوز الحدود")
			}
			break
		}
	}
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return empty, errors.New("TTML غير صالح")
		}
		if text, ok := token.(xml.CharData); ok && strings.TrimSpace(string(text)) == "" {
			continue
		}
		if _, ok := token.(xml.Comment); ok {
			continue
		}
		return empty, errors.New("TTML يحتوي بيانات إضافية غير صالحة")
	}
	var lines []WordLine
	var visit func(*lyricXMLNode, bool)
	visit = func(node *lyricXMLNode, body bool) {
		body = body || node.Name == "body"
		if body && node.Name == "p" {
			start, startOK := lyricTime(node.attr("begin"))
			end, endOK := lyricTime(node.attr("end"))
			text := strings.Join(strings.Fields(node.text()), " ")
			if !startOK || !endOK || end <= start || text == "" || song.Duration > 0 && end > song.Duration+15 {
				return
			}
			words := explicitTTMLWords(node)
			for _, word := range words {
				if word.Start < start-.01 || word.End > end+.01 {
					words = nil
					break
				}
			}
			lines = append(lines, WordLine{Start: start, End: end, Text: text, Words: words})
			return
		}
		for _, part := range node.Parts {
			if part.Child != nil {
				visit(part.Child, body)
			}
		}
	}
	visit(root, false)
	sort.SliceStable(lines, func(i, j int) bool { return lines[i].Start < lines[j].Start })
	var lrc, plain strings.Builder
	for _, line := range lines {
		milliseconds := int64(math.Round(line.Start * 1000))
		fmt.Fprintf(&lrc, "[%02d:%02d.%03d]%s\n", milliseconds/60000, milliseconds/1000%60, milliseconds%1000, line.Text)
		plain.WriteString(line.Text + "\n")
	}
	if plain.Len() == 0 {
		return empty, errors.New("TTML لا يحتوي كلمات بتوقيت صريح صالح")
	}
	record := LyricsRecord{Title: song.Title, Artist: song.Artist, Album: song.Album, Duration: song.Duration, Plain: strings.TrimSpace(plain.String())}
	if validLRC(lrc.String(), song.Duration) {
		record.Synced = lrc.String()
		record.Words = lines
	}
	return record, nil
}
