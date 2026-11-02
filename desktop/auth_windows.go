package main

import (
	"encoding/json"
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"time"
	"unsafe"
)

type privateAuth struct {
	ProviderKeys   map[string]string `json:"providerKeys,omitempty"`
	SpotifyToken   string            `json:"spotifyToken"`
	SpotifyRefresh string            `json:"spotifyRefresh"`
	Expiry         time.Time         `json:"expiry"`
	MusixmatchKey  string            `json:"musixmatchKey"`
}

func protectSecret(data []byte, decode bool) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("empty credentials")
	}
	in := windows.DataBlob{Size: uint32(len(data)), Data: &data[0]}
	var out windows.DataBlob
	var e error
	if decode {
		e = windows.CryptUnprotectData(&in, nil, nil, 0, nil, 1, &out)
	} else {
		e = windows.CryptProtectData(&in, nil, nil, 0, nil, 1, &out)
	}
	if e != nil {
		return nil, e
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte{}, unsafe.Slice(out.Data, int(out.Size))...), nil
}
func (s *Studio) saveAuth() error {
	s.mu.Lock()
	a := privateAuth{SpotifyToken: s.spotifyToken, SpotifyRefresh: s.spotifyRefresh, Expiry: s.spotifyExpiry, MusixmatchKey: s.musixmatchKey}
	a.ProviderKeys = map[string]string{}
	for provider, value := range s.metadataKeys {
		a.ProviderKeys[provider] = value
	}
	s.mu.Unlock()
	raw, e := json.Marshal(a)
	if e != nil {
		return e
	}
	sealed, e := protectSecret(raw, false)
	if e != nil {
		return e
	}
	os.MkdirAll(filepath.Join(s.root, "data"), 0755)
	return os.WriteFile(filepath.Join(s.root, "data", "credentials.dpapi"), sealed, 0600)
}
func (s *Studio) loadAuth() {
	sealed, e := os.ReadFile(filepath.Join(s.root, "data", "credentials.dpapi"))
	if e != nil {
		return
	}
	raw, e := protectSecret(sealed, true)
	if e != nil {
		return
	}
	var a privateAuth
	if json.Unmarshal(raw, &a) == nil {
		s.spotifyToken = a.SpotifyToken
		s.spotifyRefresh = a.SpotifyRefresh
		s.spotifyExpiry = a.Expiry
		s.musixmatchKey = a.MusixmatchKey
		s.metadataKeys = a.ProviderKeys
	}
}
func (s *Studio) SetMusixmatchKey(apiKey string) error {
	s.mu.Lock()
	s.musixmatchKey = apiKey
	s.mu.Unlock()
	return s.saveAuth()
}
