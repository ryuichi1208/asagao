package provider

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ryuichi1208/asagao/internal/alert"
)

func init() { Register(&Datadog{}) }

// Datadog normalizes Datadog webhook / monitor payloads.
//
// Datadog webhook payloads are fully user-defined via the notification
// message template, so there is no single canonical schema. We pick up a
// set of commonly-used field names on a best-effort basis. Operators who
// want richer data should configure their Datadog webhook template to emit
// these keys (see README).
type Datadog struct{}

func (*Datadog) Name() string { return "datadog" }

// datadogPayload mirrors the fields we attempt to read. All are optional.
type datadogPayload struct {
	// Identifiers
	ID      json.Number `json:"id"`
	AlertID json.Number `json:"alert_id"`

	// Content
	Title           string `json:"title"`
	Body            string `json:"body"`
	EventMsg        string `json:"event_msg"`
	AlertTitle      string `json:"alert_title"`
	Priority        string `json:"priority"`
	AlertType       string `json:"alert_type"`
	AlertTransition string `json:"alert_transition"`

	// Context
	Tags string `json:"tags"` // comma-separated, e.g. "env:prod,service:api"
	Org  struct {
		Name string `json:"name"`
	} `json:"org"`
}

func (*Datadog) Normalize(raw []byte) (*alert.NormalizedAlert, error) {
	var p datadogPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("datadog: invalid JSON payload: %w", err)
	}

	tags := parseDatadogTags(p.Tags)

	a := &alert.NormalizedAlert{
		Source:     "datadog",
		ID:         firstNonEmpty(string(p.AlertID), string(p.ID)),
		Title:      firstNonEmpty(p.Title, p.AlertTitle),
		Message:    firstNonEmpty(p.Body, p.EventMsg),
		Severity:   normalizeDatadogSeverity(p.Priority, p.AlertType),
		Service:    tags["service"],
		Tags:       tags,
		RawPayload: raw,
	}
	if a.Title == "" {
		a.Title = "Datadog alert"
	}
	return a, nil
}

// parseDatadogTags splits "k1:v1,k2:v2" into a map. Tags without a colon are
// stored with an empty value.
func parseDatadogTags(s string) map[string]string {
	out := map[string]string{}
	for _, t := range strings.Split(s, ",") {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if k, v, ok := strings.Cut(t, ":"); ok {
			out[strings.TrimSpace(k)] = strings.TrimSpace(v)
		} else {
			out[t] = ""
		}
	}
	return out
}

// normalizeDatadogSeverity maps Datadog's priority/alert_type onto our
// best-effort severity labels.
func normalizeDatadogSeverity(priority, alertType string) string {
	switch strings.ToLower(strings.TrimSpace(priority)) {
	case "p1", "critical":
		return "critical"
	case "p2", "p3", "warning":
		return "warning"
	}
	switch strings.ToLower(strings.TrimSpace(alertType)) {
	case "error":
		return "critical"
	case "warning":
		return "warning"
	}
	return ""
}
