// Package server exposes an HTTP webhook receiver. Each monitoring SaaS posts
// to /webhook/{provider}; the body is normalized and handed to the agent layer
// asynchronously so the webhook returns promptly (202) before the long-running
// CMA session completes.
package server

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/ryuichi1208/asagao/internal/agent"
	"github.com/ryuichi1208/asagao/internal/config"
	"github.com/ryuichi1208/asagao/internal/provider"
)

// Server wires the webhook endpoints to the agent layer.
type Server struct {
	cfg          *config.Config
	client       anthropic.Client
	agentID      string
	agentVersion int64
}

// New constructs a Server. The agent and environment must already be ensured
// (IDs present in cfg) — call agent.EnsureResources during startup.
func New(cfg *config.Config, client anthropic.Client, agentID string, agentVersion int64) *Server {
	return &Server{cfg: cfg, client: client, agentID: agentID, agentVersion: agentVersion}
}

// Handler returns the HTTP routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok\n")
	})
	mux.HandleFunc("POST /webhook/{provider}", s.handleWebhook)
	return mux
}

func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("provider")
	p, ok := provider.Get(name)
	if !ok {
		http.Error(w, fmt.Sprintf("unknown provider %q; registered: %s", name, strings.Join(provider.Names(), ", ")), http.StatusNotFound)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 5<<20)) // 5 MiB cap
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}

	a, err := p.Normalize(body)
	if err != nil {
		http.Error(w, "normalize: "+err.Error(), http.StatusBadRequest)
		return
	}

	repo, ok := s.cfg.RepoFor(a.Service)
	if !ok {
		http.Error(w, fmt.Sprintf("no repo mapping for service %q and no default", a.Service), http.StatusUnprocessableEntity)
		return
	}

	// Run the (long-lived) CMA session asynchronously; the webhook caller gets
	// an immediate 202. MVP: fire-and-log, no in-flight tracking.
	go func() {
		// Detached context: the request context is canceled when the handler
		// returns, but the session must keep running.
		ctx := context.Background()
		res, err := agent.RunFixSession(ctx, s.client, s.cfg, s.agentID, s.agentVersion, repo, os.Getenv("GITHUB_TOKEN"), a)
		if err != nil {
			log.Printf("asagao: fix session failed (provider=%s service=%s): %v", name, a.Service, err)
			return
		}
		log.Printf("asagao: fix session done (provider=%s service=%s session=%s stop=%s)", name, a.Service, res.SessionID, res.StopType)
	}()

	w.WriteHeader(http.StatusAccepted)
	_, _ = io.WriteString(w, "accepted\n")
}
