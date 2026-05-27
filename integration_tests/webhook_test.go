package integration_tests

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	ad "github.com/dakasa-yggdrasil/integration-stripe/providers/stripe/adapter"
	"github.com/dakasa-yggdrasil/integration-stripe/providers/stripe/config"
)

type recEmitter struct {
	mu    sync.Mutex
	calls []recCall
}

type recCall struct {
	routingKey string
	instance   string
	eventID    string
	eventType  string
}

func (r *recEmitter) Emit(ctx context.Context, routingKey string, env map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, recCall{
		routingKey: routingKey,
		instance:   fmt.Sprintf("%v", env["instance_id"]),
		eventID:    fmt.Sprintf("%v", env["stripe_event_id"]),
		eventType:  fmt.Sprintf("%v", env["event_type"]),
	})
	return nil
}

func sign(payload []byte, secret string, ts int64) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(fmt.Sprintf("%d.%s", ts, string(payload))))
	return fmt.Sprintf("t=%d,v1=%s", ts, hex.EncodeToString(mac.Sum(nil)))
}

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "testdata", "stripe-events", name))
	require.NoError(t, err)
	return data
}

func TestWebhook_AllFixturesRouteCorrectly(t *testing.T) {
	instances := map[string]config.InstanceConfig{
		"dakasa":    {InstanceID: "dakasa", WebhookSecret: "whsec_DAK", ToleranceSecs: 300},
		"acme-corp": {InstanceID: "acme-corp", WebhookSecret: "whsec_ACM", ToleranceSecs: 300},
	}
	rec := &recEmitter{}
	srv := ad.NewWebhookServer(zaptest.NewLogger(t), instances, ":0")
	srv.SetEmitter(rec)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	cases := []struct {
		fixture    string
		instance   string
		secret     string
		routingKey string
	}{
		{"payment_intent.succeeded.json", "dakasa", "whsec_DAK", "rta.payments.intent_succeeded"},
		{"payment_intent.payment_failed.json", "dakasa", "whsec_DAK", "rta.payments.intent_failed"},
		{"charge.refunded.json", "acme-corp", "whsec_ACM", "rta.payments.refunded"},
		{"customer.subscription.deleted.json", "dakasa", "whsec_DAK", "rta.subscriptions.cancelled"},
		{"customer.subscription.updated.json", "acme-corp", "whsec_ACM", "rta.subscriptions.updated"},
		{"account.updated.json", "dakasa", "whsec_DAK", "rta.connect.account_updated"},
	}

	now := time.Now().Unix()
	for _, c := range cases {
		body := loadFixture(t, c.fixture)
		req, _ := http.NewRequest("POST", ts.URL+"/webhooks/stripe/"+c.instance, bytes.NewReader(body))
		req.Header.Set("Stripe-Signature", sign(body, c.secret, now))
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode, "fixture %s", c.fixture)
		_ = resp.Body.Close()
	}

	// Wait for the async RTA emits to drain.
	require.Eventually(t, func() bool {
		rec.mu.Lock()
		defer rec.mu.Unlock()
		return len(rec.calls) == len(cases)
	}, 2*time.Second, 25*time.Millisecond)

	rec.mu.Lock()
	defer rec.mu.Unlock()
	require.Len(t, rec.calls, len(cases))
	// Verify each routing key appears (order may vary due to goroutine
	// emits — match by event_id ↔ case index).
	seenByEvent := make(map[string]recCall)
	for _, call := range rec.calls {
		seenByEvent[call.eventID] = call
	}
	expectedEventIDs := []string{
		"evt_pi_succeeded_01",
		"evt_pi_failed_01",
		"evt_charge_refunded_01",
		"evt_sub_deleted_01",
		"evt_sub_updated_01",
		"evt_account_updated_01",
	}
	for i, evID := range expectedEventIDs {
		got, ok := seenByEvent[evID]
		require.True(t, ok, "expected event_id %s emitted", evID)
		require.Equal(t, cases[i].routingKey, got.routingKey, "event_id %s routing", evID)
		require.Equal(t, cases[i].instance, got.instance)
	}
}

func TestWebhook_DedupesAcrossRetries(t *testing.T) {
	instances := map[string]config.InstanceConfig{
		"dakasa": {InstanceID: "dakasa", WebhookSecret: "whsec_DAK", ToleranceSecs: 300},
	}
	rec := &recEmitter{}
	srv := ad.NewWebhookServer(zaptest.NewLogger(t), instances, ":0")
	srv.SetEmitter(rec)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	body := loadFixture(t, "payment_intent.succeeded.json")
	sig := sign(body, "whsec_DAK", time.Now().Unix())
	for i := 0; i < 5; i++ {
		req, _ := http.NewRequest("POST", ts.URL+"/webhooks/stripe/dakasa", bytes.NewReader(body))
		req.Header.Set("Stripe-Signature", sig)
		resp, _ := http.DefaultClient.Do(req)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		_ = resp.Body.Close()
	}
	time.Sleep(150 * time.Millisecond)
	rec.mu.Lock()
	defer rec.mu.Unlock()
	require.Len(t, rec.calls, 1)
}

func TestWebhook_RejectsCrossTenantSecret(t *testing.T) {
	instances := map[string]config.InstanceConfig{
		"dakasa":    {InstanceID: "dakasa", WebhookSecret: "whsec_DAK", ToleranceSecs: 300},
		"acme-corp": {InstanceID: "acme-corp", WebhookSecret: "whsec_ACM", ToleranceSecs: 300},
	}
	rec := &recEmitter{}
	srv := ad.NewWebhookServer(zaptest.NewLogger(t), instances, ":0")
	srv.SetEmitter(rec)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	body := loadFixture(t, "payment_intent.succeeded.json")
	// Sign with acme secret, POST to dakasa path → must reject.
	req, _ := http.NewRequest("POST", ts.URL+"/webhooks/stripe/dakasa", bytes.NewReader(body))
	req.Header.Set("Stripe-Signature", sign(body, "whsec_ACM", time.Now().Unix()))
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	_ = resp.Body.Close()

	time.Sleep(50 * time.Millisecond)
	rec.mu.Lock()
	defer rec.mu.Unlock()
	require.Len(t, rec.calls, 0)
}
