package main

import (
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"strings"
)

func validateWorkspaceValue(k string, v json.RawMessage) error {
	bad := errors.New("قيمة غير صالحة في إعداد الواجهة: " + k)
	if string(v) == "null" {
		return bad
	}
	if strings.Contains(" motion translationEnabled translationAuto preservePitch bypass shuffle repeat normalizeVolume showOriginal showTranslation showRomanization sleepEndOfTrack fsAmbient themeEntrance watchFolders ", " "+k+" ") {
		var b bool
		if json.Unmarshal(v, &b) != nil {
			return bad
		}
		return nil
	}
	if strings.Contains(" opacity motionDuration lyricOffset volume speed reverb width preamp nawaLogo lyricScale translationScale pitch crossfade normalizationTarget fsCardSize fsTextSize fsRotation fsBlur fsDim verciLens fontSizeOriginal fontSizeTranslation ", " "+k+" ") {
		var n float64
		if json.Unmarshal(v, &n) != nil || math.IsNaN(n) || math.IsInf(n, 0) || math.Abs(n) > 100000 {
			return bad
		}
		if k == "volume" && (n < 0 || n > 1) || k == "pitch" && (n < -12 || n > 12) || k == "speed" && (n < .25 || n > 3) || k == "crossfade" && (n < 0 || n > 12) {
			return bad
		}
		return nil
	}
	switch k {
	case "eq":
		var a []float64
		if json.Unmarshal(v, &a) != nil || len(a) != 10 {
			return bad
		}
		for _, n := range a {
			if math.IsNaN(n) || math.IsInf(n, 0) || n < -24 || n > 24 {
				return bad
			}
		}
	case "favorites", "homeSections":
		var a []string
		if json.Unmarshal(v, &a) != nil || len(a) > 10000 {
			return bad
		}
		for _, x := range a {
			if len(x) > 200 {
				return bad
			}
		}
	case "playlists":
		var lists []struct {
			ID     string   `json:"id"`
			Name   string   `json:"name"`
			Tracks []string `json:"tracks"`
		}
		if json.Unmarshal(v, &lists) != nil || len(lists) > 1000 {
			return bad
		}
		for _, p := range lists {
			if p.ID == "" || len(p.ID) > 200 || len(p.Name) > 1000 || p.Tracks == nil || len(p.Tracks) > 10000 {
				return bad
			}
		}
	case "themePresets", "fullscreenPresets", "soundProfiles":
		var presets []struct {
			Name   string                     `json:"name"`
			Design map[string]json.RawMessage `json:"design"`
			Values map[string]json.RawMessage `json:"values"`
		}
		if json.Unmarshal(v, &presets) != nil || len(presets) > 100 {
			return bad
		}
		for _, p := range presets {
			if len(p.Name) > 1000 {
				return bad
			}
			values := p.Design
			if k == "soundProfiles" {
				values = p.Values
			}
			for key, raw := range values {
				if key == "themePresets" || key == "fullscreenPresets" || key == "soundProfiles" || key == "savedDesigns" {
					return bad
				}
				if e := validateWorkspaceValue(key, raw); e != nil {
					return e
				}
			}
		}
	case "savedDesigns", "playlistDesigns":
		var values map[string]map[string]json.RawMessage
		if json.Unmarshal(v, &values) != nil || len(values) > 10000 {
			return bad
		}
		for id, d := range values {
			if len(id) > 200 {
				return bad
			}
			for key, raw := range d {
				if key == "savedDesigns" || key == "themePresets" || key == "fullscreenPresets" || key == "soundProfiles" {
					return bad
				}
				if e := validateWorkspaceValue(key, raw); e != nil {
					return e
				}
			}
		}
	case "presentationBase":
 var values map[string]json.RawMessage;if json.Unmarshal(v,&values)!=nil{return bad};for key,raw:=range values{if key=="presentationBase"||key=="savedDesigns"||key=="playlistDesigns"||key=="themePresets"||key=="fullscreenPresets"||key=="soundProfiles"{return bad};if e:=validateWorkspaceValue(key,raw);e!=nil{return e}}
 case "fsVariants":
		var variants map[string]string
		if json.Unmarshal(v, &variants) != nil {
			return bad
		}
		for kind, finish := range variants {
			valid := map[string]string{"cd": " cover silver prism ", "vinyl": " grooved picture smoke ", "cassette": " classic clear neon "}[kind]
			if valid == "" || !strings.Contains(valid, " "+finish+" ") {
				return bad
			}
		}
	case "colorSets":
		var palettes map[string]map[string]string
		if json.Unmarshal(v, &palettes) != nil {
			return bad
		}
		for _, p := range palettes {
			for _, value := range p {
				if !regexp.MustCompile(`^#[0-9a-fA-F]{6}$`).MatchString(value) {
					return bad
				}
			}
		}
	case "theme", "shape", "fsMedium", "fsBackground", "fsProgress", "language", "uiDensity", "lyricsScene", "lyricReveal":
		var text string
		if json.Unmarshal(v, &text) != nil {
			return bad
		}
		allowed := map[string]string{"theme": " glass orchard pulse cloud prism noir bloodborne kingdom reload ", "shape": " soft pill sharp ", "fsMedium": " cd vinyl cassette square ", "fsBackground": " aurora blue cinema noir album theme ", "fsProgress": " wave line glow pulse shimmer segments comet ", "language": " ar en ", "uiDensity": " compact comfortable spacious ", "lyricsScene": " theme cinema karaoke focus album party ", "lyricReveal": " word character line "}[k]
		if !strings.Contains(allowed, " "+text+" ") {
			return bad
		}
	case "activePlaylist", "lyricFont", "translationFont", "lyricStyle", "lyricMotion", "lyricSource", "translationTarget", "translationSource", "performanceMode", "lyricFps", "verciText", "verciAccent", "verciSecondary":
		var text string
		if json.Unmarshal(v, &text) != nil || len(text) > 300 || strings.ContainsAny(text, "<>;{}") {
			return bad
		}
	}
	return nil
}
