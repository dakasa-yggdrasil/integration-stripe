// Package config holds per-instance Stripe adapter configuration loaded
// from Yggdrasil instance manifests. Each Stripe account = one
// integration_instance keyed by InstanceID.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// InstanceConfig is the per-instance Stripe configuration loaded from
// Yggdrasil instance_spec.config + credentials at adapter startup.
//
// APIKey + WebhookSecret are SECRETS: never log, never include in JSON
// responses, never appear in RTA envelopes. The CI grep gate in
// .github/workflows/ci.yml enforces this.
type InstanceConfig struct {
	InstanceID    string `json:"instance_id"`
	APIKey        string `json:"-"`
	WebhookSecret string `json:"-"`
	AccountID     string `json:"stripe_account_id,omitempty"`
	APIVersion    string `json:"stripe_api_version,omitempty"`
	ToleranceSecs int64  `json:"webhook_tolerance_seconds,omitempty"`
}

// WithDefaults applies the spec-mandated defaults for unset fields.
func (c InstanceConfig) WithDefaults() InstanceConfig {
	if strings.TrimSpace(c.APIVersion) == "" {
		c.APIVersion = "2024-12-18.acacia"
	}
	if c.ToleranceSecs <= 0 {
		c.ToleranceSecs = 300
	}
	return c
}

// LoadInstances iterates the configured Stripe instances from one of two
// sources:
//
//  1. STRIPE_INSTANCES_CONFIG: path to a JSON file containing
//     {"instances":[{...}, {...}]}. Used by the dakasa-system pod that
//     injects the multi-tenant instance set via a ConfigMap.
//  2. Falls back to single-tenant from envs (STRIPE_API_KEY,
//     STRIPE_WEBHOOK_SECRET, STRIPE_ACCOUNT_ID).
//
// Returns a map keyed by InstanceID so the webhook server can route by
// path segment. Errors only on malformed JSON; an empty result is
// allowed (warning emitted by the caller).
func LoadInstances(path string) (map[string]InstanceConfig, error) {
	out := make(map[string]InstanceConfig)

	if strings.TrimSpace(path) != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read instances config: %w", err)
		}
		// instanceEntry mirrors InstanceConfig but exposes secret fields
		// via JSON tags so a ConfigMap/Secret-projected file can hydrate
		// them. The runtime InstanceConfig itself keeps `json:"-"` so
		// secrets never round-trip through marshalling.
		var doc struct {
			Instances []struct {
				InstanceID    string `json:"instance_id"`
				APIKey        string `json:"stripe_api_key"`
				WebhookSecret string `json:"stripe_webhook_secret"`
				AccountID     string `json:"stripe_account_id"`
				APIVersion    string `json:"stripe_api_version"`
				ToleranceSecs int64  `json:"webhook_tolerance_seconds"`
			} `json:"instances"`
		}
		if err := json.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("parse instances config: %w", err)
		}
		for _, ic := range doc.Instances {
			if ic.InstanceID == "" {
				continue
			}
			out[ic.InstanceID] = InstanceConfig{
				InstanceID:    ic.InstanceID,
				APIKey:        ic.APIKey,
				WebhookSecret: ic.WebhookSecret,
				AccountID:     ic.AccountID,
				APIVersion:    ic.APIVersion,
				ToleranceSecs: ic.ToleranceSecs,
			}.WithDefaults()
		}
		return out, nil
	}

	// Single-tenant env fallback.
	if k := strings.TrimSpace(os.Getenv("STRIPE_API_KEY")); k != "" {
		ic := InstanceConfig{
			InstanceID:    envOrDefault("STRIPE_INSTANCE_ID", "default"),
			APIKey:        k,
			WebhookSecret: os.Getenv("STRIPE_WEBHOOK_SECRET"),
			AccountID:     os.Getenv("STRIPE_ACCOUNT_ID"),
		}.WithDefaults()
		out[ic.InstanceID] = ic
	}
	return out, nil
}

func envOrDefault(name, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return fallback
}
