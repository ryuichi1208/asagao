package provider_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ryuichi1208/asagao/internal/alert"
	"github.com/ryuichi1208/asagao/internal/provider"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		file     string
		want     alert.NormalizedAlert // RawPayload is ignored in the comparison
	}{
		{
			name:     "datadog firing",
			provider: "datadog",
			file:     "testdata/datadog.json",
			want: alert.NormalizedAlert{
				Source:   "datadog",
				ID:       "987654",
				Title:    "[Triggered] High error rate on api",
				Message:  "The error rate for service:api exceeded 5% over the last 5 minutes.",
				Severity: "critical",
				Service:  "api",
				Tags: map[string]string{
					"env":     "prod",
					"service": "api",
					"host":    "web-01",
				},
			},
		},
		{
			name:     "grafana firing",
			provider: "grafana",
			file:     "testdata/grafana.json",
			want: alert.NormalizedAlert{
				Source:   "grafana",
				ID:       "abc123def456",
				Title:    "HighLatency",
				Message:  "p99 latency for checkout exceeded 2s for 5 minutes.",
				Severity: "critical",
				Service:  "checkout",
				Tags: map[string]string{
					"env":       "prod",
					"alertname": "HighLatency",
					"service":   "checkout",
					"severity":  "critical",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, ok := provider.Get(tt.provider)
			if !ok {
				t.Fatalf("provider %q not registered", tt.provider)
			}
			raw, err := os.ReadFile(filepath.Clean(tt.file))
			if err != nil {
				t.Fatalf("read %s: %v", tt.file, err)
			}
			got, err := p.Normalize(raw)
			if err != nil {
				t.Fatalf("Normalize: %v", err)
			}

			if got.Source != tt.want.Source {
				t.Errorf("Source = %q, want %q", got.Source, tt.want.Source)
			}
			if got.ID != tt.want.ID {
				t.Errorf("ID = %q, want %q", got.ID, tt.want.ID)
			}
			if got.Title != tt.want.Title {
				t.Errorf("Title = %q, want %q", got.Title, tt.want.Title)
			}
			if got.Message != tt.want.Message {
				t.Errorf("Message = %q, want %q", got.Message, tt.want.Message)
			}
			if got.Severity != tt.want.Severity {
				t.Errorf("Severity = %q, want %q", got.Severity, tt.want.Severity)
			}
			if got.Service != tt.want.Service {
				t.Errorf("Service = %q, want %q", got.Service, tt.want.Service)
			}
			if len(got.Tags) != len(tt.want.Tags) {
				t.Errorf("Tags len = %d, want %d (%v)", len(got.Tags), len(tt.want.Tags), got.Tags)
			}
			for k, v := range tt.want.Tags {
				if got.Tags[k] != v {
					t.Errorf("Tags[%q] = %q, want %q", k, got.Tags[k], v)
				}
			}
			if len(got.RawPayload) == 0 {
				t.Error("RawPayload is empty; want original bytes retained")
			}
		})
	}
}

func TestNormalizeInvalidJSON(t *testing.T) {
	for _, name := range []string{"datadog", "grafana"} {
		p, ok := provider.Get(name)
		if !ok {
			t.Fatalf("provider %q not registered", name)
		}
		if _, err := p.Normalize([]byte("not json")); err == nil {
			t.Errorf("%s: expected error on invalid JSON, got nil", name)
		}
	}
}

func TestRegistry(t *testing.T) {
	names := provider.Names()
	want := map[string]bool{"datadog": true, "grafana": true}
	for _, n := range names {
		delete(want, n)
	}
	if len(want) != 0 {
		t.Errorf("expected datadog and grafana registered; missing %v (have %v)", want, names)
	}
}
