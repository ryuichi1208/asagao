package provider

import (
	"errors"

	"github.com/ryuichi1208/asagao/internal/alert"
)

// NewRelic is a placeholder provider for New Relic alert webhooks.
//
// TODO(newrelic): implement Normalize against the New Relic workflow webhook
// payload. The payload exposes `issueUrl`, `title`, `priority`, `state`, and
// an `issuePageUrl`, plus `entitiesData`/`tags`. Map those onto
// NormalizedAlert and register the provider via init(). Left unregistered for
// now so the registry only advertises implemented providers.
type NewRelic struct{}

func (*NewRelic) Name() string { return "newrelic" }

func (*NewRelic) Normalize(raw []byte) (*alert.NormalizedAlert, error) {
	return nil, errors.New("newrelic: provider not implemented yet")
}
