package server_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/ryuichi1208/asagao/internal/config"
	"github.com/ryuichi1208/asagao/internal/server"

	_ "github.com/ryuichi1208/asagao/internal/provider" // register providers
)

// newTestServer builds a Server with a repo mapping but no real Anthropic
// credentials. Routing, normalization, and repo resolution are exercised
// without reaching a live CMA session (the async goroutine fails and logs,
// which does not affect the synchronous response we assert on).
func newTestServer(t *testing.T) http.Handler {
	t.Helper()
	cfg := &config.Config{
		Repos: map[string]config.RepoMapping{
			"checkout": {URL: "https://github.com/x/checkout", Branch: "main"},
			"default":  {URL: "https://github.com/x/mono", Branch: "main"},
		},
	}
	s := server.New(cfg, anthropic.NewClient(), "agent_test", 1)
	return s.Handler()
}

func TestHealthz(t *testing.T) {
	h := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz status = %d, want 200", rec.Code)
	}
}

func TestWebhookUnknownProvider(t *testing.T) {
	h := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/webhook/bogus", strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown provider status = %d, want 404", rec.Code)
	}
}

func TestWebhookInvalidPayload(t *testing.T) {
	h := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/webhook/grafana", strings.NewReader("not json"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid payload status = %d, want 400", rec.Code)
	}
}

func TestWebhookAccepted(t *testing.T) {
	// A valid grafana payload whose service maps to a configured repo should
	// be accepted (202) synchronously; the async CMA run is fire-and-forget.
	body, err := os.ReadFile("../provider/testdata/grafana.json")
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}
	h := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/webhook/grafana", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("accepted status = %d, want 202 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestWebhookNoRepoMapping(t *testing.T) {
	// A payload whose service has no mapping AND with no default should 422.
	cfg := &config.Config{
		Repos: map[string]config.RepoMapping{
			"other": {URL: "https://github.com/x/other", Branch: "main"},
		},
	}
	s := server.New(cfg, anthropic.NewClient(), "agent_test", 1)
	h := s.Handler()

	body, _ := os.ReadFile("../provider/testdata/grafana.json") // service=checkout, unmapped here
	req := httptest.NewRequest(http.MethodPost, "/webhook/grafana", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("no-mapping status = %d, want 422", rec.Code)
	}
}
