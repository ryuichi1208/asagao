package provider

import (
	"encoding/json"
	"fmt"
	"maps"
	"strings"

	"github.com/ryuichi1208/asagao/internal/alert"
)

func init() { Register(&Grafana{}) }

// Grafana normalizes Grafana unified-alerting webhook payloads.
//
// The contact-point webhook posts a body containing a top-level summary plus
// an `alerts` array (the Alertmanager-style shape). We normalize the first
// firing alert (falling back to the first alert) since asagao processes one
// alert per session.
type Grafana struct{}

func (*Grafana) Name() string { return "grafana" }

type grafanaPayload struct {
	Status       string            `json:"status"` // "firing" | "resolved"
	Title        string            `json:"title"`
	Message      string            `json:"message"`
	Alerts       []grafanaAlert    `json:"alerts"`
	CommonLabels map[string]string `json:"commonLabels"`
}

type grafanaAlert struct {
	Status      string            `json:"status"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	Fingerprint string            `json:"fingerprint"`
}

func (*Grafana) Normalize(raw []byte) (*alert.NormalizedAlert, error) {
	var p grafanaPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("grafana: invalid JSON payload: %w", err)
	}

	picked := pickGrafanaAlert(p.Alerts)

	tags := map[string]string{}
	maps.Copy(tags, p.CommonLabels)
	if picked != nil {
		maps.Copy(tags, picked.Labels)
	}

	a := &alert.NormalizedAlert{
		Source:     "grafana",
		Tags:       tags,
		Service:    tags["service"],
		RawPayload: raw,
	}

	if picked != nil {
		a.ID = picked.Fingerprint
		a.Title = firstNonEmpty(picked.Labels["alertname"], picked.Annotations["summary"], p.Title)
		a.Message = firstNonEmpty(picked.Annotations["description"], picked.Annotations["summary"], p.Message)
		a.Severity = normalizeGrafanaSeverity(picked.Labels["severity"], firstNonEmpty(picked.Status, p.Status))
	} else {
		a.Title = p.Title
		a.Message = p.Message
		a.Severity = normalizeGrafanaSeverity("", p.Status)
	}
	if a.Title == "" {
		a.Title = "Grafana alert"
	}
	return a, nil
}

// pickGrafanaAlert returns the first firing alert, or the first alert if none
// are firing, or nil if the array is empty.
func pickGrafanaAlert(alerts []grafanaAlert) *grafanaAlert {
	for i := range alerts {
		if strings.EqualFold(alerts[i].Status, "firing") {
			return &alerts[i]
		}
	}
	if len(alerts) > 0 {
		return &alerts[0]
	}
	return nil
}

func normalizeGrafanaSeverity(severityLabel, status string) string {
	switch strings.ToLower(strings.TrimSpace(severityLabel)) {
	case "critical", "crit", "page":
		return "critical"
	case "warning", "warn":
		return "warning"
	case "info":
		return "info"
	}
	if strings.EqualFold(status, "resolved") {
		return "resolved"
	}
	return ""
}
