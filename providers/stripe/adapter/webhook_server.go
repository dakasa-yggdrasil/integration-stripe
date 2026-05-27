package adapter

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/dakasa-yggdrasil/integration-stripe/providers/stripe/config"
)

// dedupTTL is the in-memory dedup window. Spec §2 mandates 24h.
const dedupTTL = 24 * time.Hour

// dedupEntry is one row in the dedup map.
type dedupEntry struct {
	at time.Time
}

// WebhookServer routes inbound Stripe deliveries by instance_id (path
// segment of /webhooks/stripe/{instance_id}). Built independently of
// the SDK RPC mux so the reactor surface stays separate from the
// execute/describe endpoints.
type WebhookServer struct {
	logger    *zap.Logger
	instances map[string]config.InstanceConfig
	emitter   RTAEmitter
	dedup     sync.Map // event_id -> dedupEntry
	srv       *http.Server
}

// RTAEmitter abstracts the RTA publish path so tests can swap it for a
// recorder. Production wiring binds it to the SDK's RTA emit (or, in
// the interim, calls integration-rabbitmq publish_message capability).
type RTAEmitter interface {
	Emit(ctx context.Context, routingKey string, envelope map[string]any) error
}

// NewWebhookServer constructs the reactor server bound to addr but does
// NOT start listening until ListenAndServe is called.
func NewWebhookServer(logger *zap.Logger, instances map[string]config.InstanceConfig, addr string) *WebhookServer {
	mux := http.NewServeMux()
	s := &WebhookServer{
		logger:    logger,
		instances: instances,
		srv: &http.Server{
			Addr:              addr,
			Handler:           mux,
			ReadHeaderTimeout: 10 * time.Second,
		},
	}
	mux.HandleFunc("/webhooks/stripe/", s.handleStripeWebhook)
	return s
}

// SetEmitter wires an RTA publisher; tests inject a recorder.
func (s *WebhookServer) SetEmitter(e RTAEmitter) { s.emitter = e }

// Handler returns the underlying mux so integration tests can wrap it
// in httptest.NewServer.
func (s *WebhookServer) Handler() http.Handler { return s.srv.Handler }

// ListenAndServe binds the listen socket and serves until Shutdown is
// invoked.
func (s *WebhookServer) ListenAndServe() error { return s.srv.ListenAndServe() }

// Shutdown gracefully stops the server.
func (s *WebhookServer) Shutdown(ctx context.Context) error { return s.srv.Shutdown(ctx) }

// handleStripeWebhook implements the full reactor flow:
//
//  1. Extract instance_id from the path.
//  2. Read raw body (max 65536 bytes).
//  3. Verify HMAC against the instance's webhook secret.
//  4. Parse event ID/type.
//  5. Check dedup; if hit, return 200.
//  6. Record dedup entry.
//  7. Return 200 to Stripe (BEFORE emit, per spec §2.5).
//  8. Emit RTA envelope asynchronously.
//
// The 200 is sent BEFORE the emit so a slow downstream RTA path does
// not trigger Stripe's retry storm. Emit failures log but never
// influence the HTTP response.
func (s *WebhookServer) handleStripeWebhook(w http.ResponseWriter, r *http.Request) {
	instanceID := strings.TrimPrefix(r.URL.Path, "/webhooks/stripe/")
	if instanceID == "" {
		http.Error(w, "instance_id required", http.StatusBadRequest)
		return
	}
	inst, ok := s.instances[instanceID]
	if !ok {
		s.logger.Warn("webhook for unknown instance",
			zap.String("instance_id", instanceID))
		http.Error(w, "unknown instance", http.StatusNotFound)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 65536))
	if err != nil {
		http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
		return
	}

	sigHeader := r.Header.Get("Stripe-Signature")
	if _, err := VerifySignature(body, sigHeader, []byte(inst.WebhookSecret), inst.ToleranceSecs); err != nil {
		s.logger.Warn("invalid webhook signature",
			zap.String("instance_id", instanceID),
			zap.Error(err))
		StripeWebhookSigFailures.WithLabelValues(instanceID).Inc()
		http.Error(w, "invalid signature", http.StatusBadRequest)
		return
	}

	var ev struct {
		ID       string          `json:"id"`
		Type     string          `json:"type"`
		LiveMode bool            `json:"livemode"`
		Data     json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &ev); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	dedupKey := instanceID + ":" + ev.ID
	if _, dup := s.dedup.LoadOrStore(dedupKey, dedupEntry{at: time.Now()}); dup {
		StripeWebhookDedup.WithLabelValues(instanceID).Inc()
		w.WriteHeader(http.StatusOK)
		return
	}
	StripeWebhookReceived.WithLabelValues(ev.Type, instanceID).Inc()
	go s.evictExpired()

	// 200 BEFORE emit (spec §2.5).
	w.WriteHeader(http.StatusOK)

	routingKey := eventTypeToRTAKey(ev.Type)
	envelope := map[string]any{
		"routing_key":     routingKey,
		"instance_id":     instanceID,
		"stripe_event_id": ev.ID,
		"event_type":      ev.Type,
		"livemode":        ev.LiveMode,
		"payload":         json.RawMessage(ev.Data),
	}
	if s.emitter != nil {
		// Decouple the emit from the request lifecycle — the response
		// has already been written, so a long emit shouldn't hold the
		// HTTP connection open.
		go func() {
			emitCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := s.emitter.Emit(emitCtx, routingKey, envelope); err != nil {
				s.logger.Error("rta emit failed",
					zap.String("instance_id", instanceID),
					zap.String("routing_key", routingKey),
					zap.Error(err))
				StripeRTAEmitErrors.WithLabelValues(routingKey, instanceID).Inc()
			} else {
				StripeRTAEmit.WithLabelValues(routingKey, instanceID).Inc()
			}
		}()
	}
}

func (s *WebhookServer) evictExpired() {
	now := time.Now()
	s.dedup.Range(func(k, v any) bool {
		if e, ok := v.(dedupEntry); ok && now.Sub(e.at) > dedupTTL {
			s.dedup.Delete(k)
		}
		return true
	})
}
