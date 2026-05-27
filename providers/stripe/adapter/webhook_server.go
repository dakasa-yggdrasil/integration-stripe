package adapter

import (
	"net/http"

	"go.uber.org/zap"

	"github.com/dakasa-yggdrasil/integration-stripe/providers/stripe/config"
)

// NewWebhookServer is a stub for Task 7 bootstrap. Task 33 replaces it
// with a full *WebhookServer implementing HMAC verify + dedup + RTA
// emit.
func NewWebhookServer(logger *zap.Logger, instances map[string]config.InstanceConfig, addr string) *http.Server {
	mux := http.NewServeMux()
	return &http.Server{Addr: addr, Handler: mux}
}
