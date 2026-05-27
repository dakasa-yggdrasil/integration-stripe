package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/dakasa-yggdrasil/yggdrasil-sdk-go/adapter"
	"go.uber.org/zap"

	ad "github.com/dakasa-yggdrasil/integration-stripe/providers/stripe/adapter"
	"github.com/dakasa-yggdrasil/integration-stripe/providers/stripe/config"
	"github.com/dakasa-yggdrasil/integration-stripe/providers/stripe/message"
)

// main bootstraps the stripe adapter with 3 listeners:
//
//   - RPC on ADAPTER_PORT (default 8081): /rpc/describe + /rpc/execute
//     via yggdrasil-sdk-go adapter.New(...).ListenHTTP.
//   - Webhook on WEBHOOK_PORT (default 8082): /webhooks/stripe/{instance_id}
//     served by webhook_server.go separately from the SDK mux.
//   - Health on HEALTHCHECK_PORT (default 8080): /healthz + /readyz.
func main() {
	logger, err := zap.NewProduction()
	if err != nil {
		panic(err)
	}
	defer func() { _ = logger.Sync() }()

	instances, err := config.LoadInstances(os.Getenv("STRIPE_INSTANCES_CONFIG"))
	if err != nil {
		logger.Fatal("load instances", zap.Error(err))
	}
	logger.Info("loaded stripe instances", zap.Int("count", len(instances)))

	a := adapter.New(adapter.Config{
		Provider:        ad.Provider,
		IntegrationType: ad.IntegrationType,
		Version:         ad.AdapterVersion,
		DefaultTimeout:  30 * time.Second,
		Concurrency:     5,
	}).
		Register("describe", message.DescribeHandler(logger)).
		Register("execute", message.ExecuteHandler(logger, instances))

	rpcAddr := ":" + envOrDefault("ADAPTER_PORT", "8081")
	a.ListenHTTP(rpcAddr)
	logger.Info("rpc listener", zap.String("addr", rpcAddr))

	whSrv := ad.NewWebhookServer(logger, instances, ":"+envOrDefault("WEBHOOK_PORT", "8082"))
	go func() {
		if err := whSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal("webhook server", zap.Error(err))
		}
	}()

	healthSrv := newHealthServer()
	go func() {
		if err := healthSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal("health server", zap.Error(err))
		}
	}()

	ctx := adapter.WithSignalHandler(context.Background())
	if err := a.Run(ctx); err != nil {
		logger.Fatal("adapter run", zap.Error(err))
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = whSrv.Shutdown(shutdownCtx)
	_ = healthSrv.Shutdown(shutdownCtx)
}

func newHealthServer() *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	})
	return &http.Server{
		Addr:              ":" + envOrDefault("HEALTHCHECK_PORT", "8080"),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
}

func envOrDefault(name, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return fallback
}
