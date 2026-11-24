package adapter

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dakasa-yggdrasil/integration-stripe/providers/stripe/config"
	"go.uber.org/zap"
)

type originalBlockedRTA struct {
	entered chan context.Context
	release chan struct{}
	calls   atomic.Int32
}

func (e *originalBlockedRTA) Emit(ctx context.Context, _ string, _ map[string]any) error {
	e.calls.Add(1)
	e.entered <- ctx
	<-e.release
	return ctx.Err()
}

func TestWebhookShutdownJoinsOriginalAsyncEmitAfterResponseWithoutCancellingIt(t *testing.T) {
	const secret = "whsec_ci_original"
	server := NewWebhookServer(zap.NewNop(), map[string]config.InstanceConfig{
		"fixture": {InstanceID: "fixture", WebhookSecret: secret, ToleranceSecs: 300},
	}, "127.0.0.1:0")
	emitter := &originalBlockedRTA{entered: make(chan context.Context, 1), release: make(chan struct{})}
	server.SetEmitter(emitter)
	ts := httptest.NewUnstartedServer(server.Handler())
	server.srv = ts.Config // Shutdown owns this actual HTTP listener too.
	ts.Start()
	var once sync.Once
	release := func() { once.Do(func() { close(emitter.release) }) }
	defer ts.Close()
	defer release()
	payload := []byte(`{"id":"evt_original","type":"payment_intent.succeeded","data":{}}`)
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/webhooks/stripe/fixture", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Stripe-Signature", makeStripeSig(payload, []byte(secret), time.Now().Unix()))
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatal("original response-before-emit contract changed")
	}
	var original context.Context
	select {
	case original = <-emitter.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("original emit never started")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := server.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("blocked original emit became a qualified drain: %v", err)
	}
	if original.Err() != nil {
		t.Fatal("shutdown cancelled the admitted emitter context")
	}
	late := httptest.NewRecorder()
	server.Handler().ServeHTTP(late, httptest.NewRequest(http.MethodPost, "/webhooks/stripe/fixture", bytes.NewReader(payload)))
	if late.Code != http.StatusServiceUnavailable || emitter.calls.Load() != 1 {
		t.Fatal("new webhook admitted after original shutdown")
	}
	first, second := make(chan error, 1), make(chan error, 1)
	go func() { first <- server.Shutdown(context.Background()) }()
	go func() { second <- server.Shutdown(context.Background()) }()
	select {
	case <-first:
		t.Fatal("concurrent caller escaped the original emit")
	case <-second:
		t.Fatal("concurrent caller escaped the original emit")
	case <-time.After(25 * time.Millisecond):
	}
	release()
	for _, done := range []<-chan error{first, second} {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("same original emit did not join")
		}
	}
	if emitter.calls.Load() != 1 {
		t.Fatal("original emit replayed")
	}
}
