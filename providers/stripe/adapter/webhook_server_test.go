package adapter

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/dakasa-yggdrasil/integration-stripe/providers/stripe/config"
)

// recordingEmitter captures every RTA emit so tests can assert on
// routing key + envelope shape.
type recordingEmitter struct {
	mu    sync.Mutex
	calls []recordedCall
}
type recordedCall struct {
	routingKey string
	envelope   map[string]any
}

func (r *recordingEmitter) Emit(ctx context.Context, routingKey string, envelope map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, recordedCall{routingKey: routingKey, envelope: envelope})
	return nil
}

func TestReactor_MultiTenant_InstanceA_RejectsInstanceBSecret(t *testing.T) {
	instances := map[string]config.InstanceConfig{
		"a": {InstanceID: "a", WebhookSecret: "whsec_AAA", ToleranceSecs: 300},
		"b": {InstanceID: "b", WebhookSecret: "whsec_BBB", ToleranceSecs: 300},
	}
	rec := &recordingEmitter{}
	srv := NewWebhookServer(zaptest.NewLogger(t), instances, ":0")
	srv.SetEmitter(rec)

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	payload := []byte(`{"id":"evt_AAA","type":"payment_intent.succeeded","livemode":false,"data":{}}`)
	now := time.Now().Unix()

	// Signed with B's secret → POSTing to A's path = bad signature → 400.
	badSig := makeStripeSig(payload, []byte("whsec_BBB"), now)
	req, _ := http.NewRequest("POST", fmt.Sprintf("%s/webhooks/stripe/a", ts.URL), bytes.NewReader(payload))
	req.Header.Set("Stripe-Signature", badSig)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	_ = resp.Body.Close()

	// Same payload signed with A's secret → 200 + emit recorded.
	goodSig := makeStripeSig(payload, []byte("whsec_AAA"), now)
	req2, _ := http.NewRequest("POST", fmt.Sprintf("%s/webhooks/stripe/a", ts.URL), bytes.NewReader(payload))
	req2.Header.Set("Stripe-Signature", goodSig)
	resp2, err := http.DefaultClient.Do(req2)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp2.StatusCode)
	_ = resp2.Body.Close()

	// Wait briefly for the goroutine RTA emit.
	time.Sleep(100 * time.Millisecond)

	rec.mu.Lock()
	defer rec.mu.Unlock()
	require.Len(t, rec.calls, 1)
	require.Equal(t, "rta.payments.intent_succeeded", rec.calls[0].routingKey)
	require.Equal(t, "a", rec.calls[0].envelope["instance_id"])
	require.Equal(t, "evt_AAA", rec.calls[0].envelope["stripe_event_id"])
}

func TestReactor_DedupesSameEventID(t *testing.T) {
	instances := map[string]config.InstanceConfig{
		"a": {InstanceID: "a", WebhookSecret: "whsec_AAA", ToleranceSecs: 300},
	}
	rec := &recordingEmitter{}
	srv := NewWebhookServer(zaptest.NewLogger(t), instances, ":0")
	srv.SetEmitter(rec)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	payload := []byte(`{"id":"evt_DUP","type":"payment_intent.succeeded","livemode":false,"data":{}}`)
	sig := makeStripeSig(payload, []byte("whsec_AAA"), time.Now().Unix())

	for i := 0; i < 3; i++ {
		req, _ := http.NewRequest("POST", ts.URL+"/webhooks/stripe/a", bytes.NewReader(payload))
		req.Header.Set("Stripe-Signature", sig)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		_ = resp.Body.Close()
	}

	time.Sleep(100 * time.Millisecond)
	rec.mu.Lock()
	defer rec.mu.Unlock()
	require.Len(t, rec.calls, 1, "dedup must suppress repeats of evt_DUP")
}
