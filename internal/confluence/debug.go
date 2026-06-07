package confluence

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	pkgconfluence "github.com/ekinolik/jax/pkg/confluence"
)

// StartDebugServer serves GET /confluence/debug?ticker=SYMBOL on addr (e.g. ":8081").
// Returns 200 with snapshot JSON or 404 when no snapshot exists.
func StartDebugServer(addr string, processor *Processor) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/confluence/debug", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		ticker := pkgconfluence.NormalizeTicker(r.URL.Query().Get("ticker"))
		if err := pkgconfluence.ValidateTicker(ticker); err != nil {
			http.Error(w, "invalid ticker", http.StatusBadRequest)
			return
		}
		snap, ok := processor.GetSnapshot(ticker)
		if !ok {
			http.Error(w, "no snapshot for ticker", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if err := enc.Encode(snap); err != nil {
			http.Error(w, "encode error", http.StatusInternalServerError)
		}
	})

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		log.Printf("[confluence] debug server listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[confluence] debug server: %v", err)
		}
	}()
	return srv
}

// DebugPortFromEnv returns JAX_CONFLUENCE_DEBUG_PORT or empty to disable debug HTTP.
func DebugPortFromEnv(portStr string) string {
	portStr = strings.TrimSpace(portStr)
	if portStr == "" {
		return ""
	}
	if _, err := strconv.Atoi(portStr); err != nil {
		return ""
	}
	return "127.0.0.1:" + portStr
}
