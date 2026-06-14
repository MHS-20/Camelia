package node

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
)

type HTTPServer struct {
	fs     *FileServer
	server *http.Server
}

func NewHTTPServer(fs *FileServer, addr string) *HTTPServer {
	h := &HTTPServer{fs: fs}
	mux := http.NewServeMux()
	mux.HandleFunc("/get", h.handleGet)
	mux.HandleFunc("/store", h.handleStore)
	mux.HandleFunc("/stats", h.handleStats)
	mux.HandleFunc("/peers", h.handlePeers)
	h.server = &http.Server{
		Addr:    addr,
		Handler: mux,
	}
	return h
}

func (h *HTTPServer) Start() error {
	log.Printf("HTTP API listening on %s", h.server.Addr)
	go func() {
		if err := h.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()
	return nil
}

func (h *HTTPServer) Stop() error {
	return h.server.Close()
}

func (h *HTTPServer) handleGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	key := r.URL.Query().Get("key")
	if key == "" {
		http.Error(w, "missing key parameter", http.StatusBadRequest)
		return
	}
	reader, size, err := h.fs.Get(key)
	if err != nil {
		http.Error(w, fmt.Sprintf("get failed: %v", err), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Length", fmt.Sprintf("%d", size))
	w.WriteHeader(http.StatusOK)
	io.Copy(w, reader)
}

func (h *HTTPServer) handleStore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	key := r.URL.Query().Get("key")
	if key == "" {
		http.Error(w, "missing key parameter", http.StatusBadRequest)
		return
	}
	if err := h.fs.Store(key, r.Body); err != nil {
		http.Error(w, fmt.Sprintf("store failed: %v", err), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("stored"))
}

func (h *HTTPServer) handleStats(w http.ResponseWriter, r *http.Request) {
	stats := h.fs.Stats()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (h *HTTPServer) handlePeers(w http.ResponseWriter, r *http.Request) {
	h.fs.peerLock.Lock()
	addrs := make([]string, 0, len(h.fs.peers))
	for addr := range h.fs.peers {
		addrs = append(addrs, addr)
	}
	h.fs.peerLock.Unlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(addrs)
}
