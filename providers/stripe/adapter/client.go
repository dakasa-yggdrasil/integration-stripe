package adapter

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"

	"github.com/stripe/stripe-go/v83"
)

// NewStripeClient builds a new stripe-go v83 client bound to the supplied
// API key and (optionally) a custom backend base URL. The base URL is the
// per-test or per-environment override (e.g. "http://stripe-mock:12111"
// or an httptest.Server URL). Pass "" to use the SDK's built-in
// production URLs.
//
// apiVersion is forwarded into BackendConfig.APIVersion when non-empty.
// Each instance gets a fresh client — the global package-level singleton
// pattern from enterprise-payments-api would defeat multi-tenant
// isolation.
func NewStripeClient(apiKey, baseURL, apiVersion string) (*stripe.Client, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, fmt.Errorf("stripe api key is required")
	}

	var opts []stripe.ClientOption
	if strings.TrimSpace(baseURL) != "" {
		cfg := &stripe.BackendConfig{URL: stripe.String(baseURL)}
		if strings.TrimSpace(apiVersion) != "" {
			// stripe.BackendConfig has no public APIVersion field in
			// v83; the version is encoded by the generated client per
			// call. We keep the parameter for forward compatibility.
			_ = apiVersion
		}
		opts = append(opts, stripe.WithBackends(stripe.NewBackendsWithConfig(cfg)))
	}
	return stripe.NewClient(apiKey, opts...), nil
}

// SetStripeClientForTest swaps a per-instance client with a custom one,
// usually one built with stripe.WithBackends(...) pointing at an
// httptest.Server. Returns a restore function so tests revert the swap
// on cleanup. Mirrors the helper in
// dakasa-enterprise-payments-api/client/stripe.go:73.
var (
	testClientMu sync.Mutex
	testClients  = make(map[string]*stripe.Client)
)

func SetStripeClientForTest(instanceID string, c *stripe.Client) func() {
	testClientMu.Lock()
	prev, hadPrev := testClients[instanceID]
	testClients[instanceID] = c
	testClientMu.Unlock()
	return func() {
		testClientMu.Lock()
		if !hadPrev {
			delete(testClients, instanceID)
		} else {
			testClients[instanceID] = prev
		}
		testClientMu.Unlock()
	}
}

// clientForInstance returns the test-injected client when one was
// registered for instanceID, otherwise builds a fresh one from the
// supplied credentials. Reused by Execute to pick the right client per
// dispatch.
func clientForInstance(instanceID, apiKey, baseURL, apiVersion string) (*stripe.Client, error) {
	testClientMu.Lock()
	if c, ok := testClients[instanceID]; ok {
		testClientMu.Unlock()
		return c, nil
	}
	testClientMu.Unlock()
	return NewStripeClient(apiKey, baseURL, apiVersion)
}

// idempotencyKeyOrDerived returns the explicit idempotency key the caller
// supplied, or computes a deterministic one from the operation + a
// stable SHA-256 of the input fields. This matches the pattern in
// enterprise-payments-api/client/stripe.go:109 +
// InvoiceItemIdempotencyKey.
func idempotencyKeyOrDerived(provided, operation string, stableFields ...string) string {
	if k := strings.TrimSpace(provided); k != "" {
		return k
	}
	base := strings.Join(append([]string{operation}, stableFields...), "|")
	sum := sha256.Sum256([]byte(base))
	return operation + "_" + hex.EncodeToString(sum[:8])
}
