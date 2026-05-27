package adapter

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// 11 Prometheus series exposed on the health server's /metrics endpoint
// (spec §11). Labels are bounded — the {capability, event_type,
// routing_key, status_code, stripe_error_type, instance} cardinality
// stays manageable because:
//   - capability + routing_key + event_type are closed sets (14 + 19 + 19)
//   - status_code is the small set of HTTP codes Stripe returns
//   - instance is bounded by the InstanceConfig map (~2-10 per deployment)
var (
	StripeRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "stripe_request_duration_seconds",
		Help:    "Stripe API call duration",
		Buckets: prometheus.ExponentialBuckets(0.01, 2, 10),
	}, []string{"op", "instance"})

	StripeRequestErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "stripe_request_errors_total",
		Help: "Stripe API call errors",
	}, []string{"op", "status_code", "stripe_error_type", "instance"})

	StripeWebhookReceived = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "stripe_webhook_received_total",
		Help: "Stripe webhook deliveries received",
	}, []string{"event_type", "instance"})

	StripeWebhookSigFailures = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "stripe_webhook_signature_failures_total",
		Help: "Webhook signature verification failures",
	}, []string{"instance"})

	StripeWebhookDedup = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "stripe_webhook_dedup_total",
		Help: "Webhook deliveries suppressed by dedup",
	}, []string{"instance"})

	StripeRTAEmit = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "stripe_rta_emit_total",
		Help: "Successful RTA envelope emissions",
	}, []string{"routing_key", "instance"})

	StripeRTAEmitErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "stripe_rta_emit_errors_total",
		Help: "Failed RTA envelope emissions",
	}, []string{"routing_key", "instance"})

	StripeExecuteRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "stripe_execute_requests_total",
		Help: "Execute capability invocations",
	}, []string{"capability", "instance"})

	StripeExecuteDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "stripe_execute_duration_seconds",
		Help:    "Execute capability duration",
		Buckets: prometheus.ExponentialBuckets(0.01, 2, 10),
	}, []string{"capability", "instance"})

	StripeAPIKeyValid = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "stripe_api_key_valid",
		Help: "1 if last background probe found the instance API key valid, 0 otherwise",
	}, []string{"instance"})

	StripeDedupMapSize = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "stripe_dedup_map_size",
		Help: "Number of dedup entries currently in memory",
	}, []string{"instance"})
)
