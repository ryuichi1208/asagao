package provider

import (
	"errors"

	"github.com/ryuichi1208/asagao/internal/alert"
)

// Mackerel is a placeholder provider for Mackerel alert webhooks.
//
// TODO(mackerel): implement Normalize against the Mackerel alert webhook
// payload (https://mackerel.io/docs/entry/howto/alerts/webhook). The payload
// exposes `alert` (id, status, monitorName, metricLabel, ...), `host`, and
// `org`. Map those onto NormalizedAlert and remove the registration guard
// below. Left unregistered for now so the registry only advertises
// implemented providers.
type Mackerel struct{}

func (*Mackerel) Name() string { return "mackerel" }

func (*Mackerel) Normalize(raw []byte) (*alert.NormalizedAlert, error) {
	return nil, errors.New("mackerel: provider not implemented yet")
}
