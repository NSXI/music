package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (s *Studio) ensureSpotifyToken(ctx context.Context) error {
	s.oauthMu.Lock()
	defer s.oauthMu.Unlock()
	s.mu.Lock()
	access, refresh, expiry, client := s.spotifyToken, s.spotifyRefresh, s.spotifyExpiry, s.state.Preferences.SpotifyClientID
	s.mu.Unlock()
	if access == "" {
		return errors.New("اربط تطبيق Spotify المستقل من الإعدادات، أو استخدم الرابط العام بدون حساب")
	}
	if expiry.IsZero() || time.Until(expiry) > 30*time.Second {
		return nil
	}
	if refresh == "" || client == "" {
		return errors.New("انتهى تصريح Spotify؛ أعد ربط الحساب المستقل")
	}
	params := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}, "client_id": {client}}
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://accounts.spotify.com/api/token", strings.NewReader(params.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, e := s.client.Do(req)
	if e != nil {
		return errors.New("تعذر تجديد تصريح Spotify؛ تحقق من الاتصال")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("تعذر تجديد تصريح Spotify (HTTP %d)", res.StatusCode)
	}
	var token struct {
		Access  string `json:"access_token"`
		Refresh string `json:"refresh_token"`
		Expires int    `json:"expires_in"`
	}
	if e = json.NewDecoder(res.Body).Decode(&token); e != nil {
		return e
	}
	if token.Access == "" {
		return errors.New("لم يرجع Spotify تصريحًا جديدًا")
	}
	s.mu.Lock()
	s.spotifyToken = token.Access
	if token.Refresh != "" {
		s.spotifyRefresh = token.Refresh
	}
	s.spotifyExpiry = time.Now().Add(time.Duration(token.Expires) * time.Second)
	s.mu.Unlock()
	return s.saveAuth()
}
