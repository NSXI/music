package main

import (
	"net"
	"net/http"
	"strings"
	"time"
)

// A loopback stream server keeps long audio responses progressive, independent
// of the WebView asset bridge's buffering. Random tokens identify each stream.
func (s *Studio) startPreviewServer() {
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		s.previewMu.Lock()
		s.previewServerError = e.Error()
		s.previewMu.Unlock()
		return
	}
	server := &http.Server{ReadHeaderTimeout: 10 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && origin != "http://wails.localhost" && origin != "https://wails.localhost" {
			http.Error(w, "Origin not allowed", 403)
			return
		}
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Expose-Headers", "Content-Range, Accept-Ranges, Content-Length")
		w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Range, If-Range")
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) != 2 || parts[0] != "stream" {
			http.NotFound(w, r)
			return
		}
		s.servePreview(w, r, parts[1])
	})}
	s.previewMu.Lock()
	s.previewServer = server
	s.previewAddress = "http://" + listener.Addr().String()
	s.previewMu.Unlock()
	go server.Serve(listener)
}
