package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/cryptoquantumwave/khunquant/pkg/config"
)

type devMCPStatus struct {
	Enabled  bool   `json:"enabled"`
	Endpoint string `json:"endpoint,omitempty"`
	Token    string `json:"token,omitempty"`
}

func (h *Handler) registerDevMCPStatusRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/dev-mcp/status", h.handleDevMCPStatus)
}

func (h *Handler) handleDevMCPStatus(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		http.Error(w, "config load error", http.StatusInternalServerError)
		return
	}
	status := devMCPStatus{
		Enabled: cfg.Debug.DevMCP.Enabled,
	}
	if cfg.Debug.DevMCP.Enabled {
		port := cfg.Gateway.Port
		prefix := cfg.Debug.DevMCP.PathPrefix
		status.Endpoint = fmt.Sprintf("http://127.0.0.1:%d%s", port, prefix)
		status.Token = cfg.Debug.DevMCP.Token
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}
