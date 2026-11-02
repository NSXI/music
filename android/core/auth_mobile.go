package main

import (
	"encoding/json"
	"errors"
	"crypto/aes"
 "crypto/cipher"
 "crypto/rand"
 "encoding/base64"
 "io"
	"os"
	"path/filepath"
	"time"
)

type privateAuth struct {
	ProviderKeys   map[string]string `json:"providerKeys,omitempty"`
	SpotifyToken   string            `json:"spotifyToken"`
	SpotifyRefresh string            `json:"spotifyRefresh"`
	Expiry         time.Time         `json:"expiry"`
	MusixmatchKey  string            `json:"musixmatchKey"`
}

var mobileSecret []byte
func protectSecret(data []byte,decode bool)([]byte,error){
 if len(mobileSecret)!=32{return nil,errors.New("Android secure key is unavailable")}
 block,e:=aes.NewCipher(mobileSecret);if e!=nil{return nil,e};g,e:=cipher.NewGCM(block);if e!=nil{return nil,e}
 if decode{if len(data)<g.NonceSize(){return nil,errors.New("invalid credentials")};return g.Open(nil,data[:g.NonceSize()],data[g.NonceSize():],nil)}
 nonce:=make([]byte,g.NonceSize());if _,e=io.ReadFull(rand.Reader,nonce);e!=nil{return nil,e};return g.Seal(nonce,nonce,data,nil),nil
}
func configureSecret(value string)error{key,e:=base64.StdEncoding.DecodeString(value);if e!=nil||len(key)!=32{return errors.New("invalid Android vault key")};mobileSecret=key;return nil}
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
	return os.WriteFile(filepath.Join(s.root, "data", "credentials.android.aes"), sealed, 0600)
}
func (s *Studio) loadAuth() {
	sealed, e := os.ReadFile(filepath.Join(s.root, "data", "credentials.android.aes"))
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
