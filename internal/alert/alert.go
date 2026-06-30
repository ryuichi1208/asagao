// Package alert defines the normalized alert representation shared across
// the ingest layer (providers) and the agent layer.
package alert

// NormalizedAlert is the provider-agnostic representation of a monitoring
// alert. Every Provider converts its source-specific payload into this shape
// so that the agent layer has a single contract to work against.
type NormalizedAlert struct {
	// Source identifies the originating provider, e.g. "datadog", "grafana".
	Source string
	// ID is the alert/monitor identifier on the SaaS side, when available.
	ID string
	// Title is a short, human-readable headline for the alert.
	Title string
	// Message is the alert body / description.
	Message string
	// Severity is a best-effort severity label, e.g. "critical", "warning".
	Severity string
	// Service is the related service name, when the payload exposes one.
	Service string
	// Tags carries arbitrary metadata (host, env, monitor_id, ...).
	Tags map[string]string
	// RawPayload retains the original payload bytes for auditing.
	RawPayload []byte
}
