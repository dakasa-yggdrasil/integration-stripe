package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/dakasa-yggdrasil/yggdrasil-sdk-go/adapter"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	ad "github.com/dakasa-yggdrasil/integration-stripe/providers/stripe/adapter"
	"github.com/dakasa-yggdrasil/integration-stripe/providers/stripe/config"
	"github.com/dakasa-yggdrasil/integration-stripe/providers/stripe/message"
)

// main bootstraps the stripe adapter with 3 listeners:
//
//   - RPC over HTTP on ADAPTER_PORT (default 8081), or over AMQP when
//     YGGDRASIL_TRANSPORT=amqp, via yggdrasil-sdk-go.
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
	})

	// Wire SDK reconcile dispatch table BEFORE the legacy "execute"
	// Register call so the ExecuteHandler can delegate operation
	// routing to reconcile.Dispatch, activating §6.5 mutation event
	// auto-emission for production traffic (not just tests).
	// adapter.Adapter.Register is last-write-wins — keeping the
	// bridge handler installed below as the single execute owner.
	ad.WireReconcilers(a, defaultInstanceID(instances))

	a.Register("describe", message.DescribeHandler(logger)).
		Register("execute", message.ExecuteHandler(logger, a, instances))

	switch transport := strings.ToLower(strings.TrimSpace(os.Getenv("YGGDRASIL_TRANSPORT"))); transport {
	case "", "http", "http_json":
		rpcAddr := ":" + envOrDefault("ADAPTER_PORT", "8081")
		a.ListenHTTP(rpcAddr)
		logger.Info("rpc listener", zap.String("transport", "http_json"), zap.String("addr", rpcAddr))
	case "amqp", "rabbitmq":
		brokerURL := strings.TrimSpace(os.Getenv("BROKER_URL"))
		if brokerURL == "" {
			logger.Fatal("YGGDRASIL_TRANSPORT=amqp but BROKER_URL is empty")
		}
		a.ListenAMQP(brokerURL)
		logger.Info("rpc listener", zap.String("transport", "rabbitmq"))
	default:
		logger.Fatal("unsupported YGGDRASIL_TRANSPORT", zap.String("value", transport))
	}

	ctx, stopAdmission := context.WithCancel(adapter.WithSignalHandler(context.Background()))
	defer stopAdmission()

	whSrv := ad.NewWebhookServer(logger, instances, ":"+envOrDefault("WEBHOOK_PORT", "8082"))
	webhookFailure := serveSideListener(whSrv.ListenAndServe, logger, stopAdmission)

	healthSrv := newHealthServer()
	healthFailure := serveHealthServer(healthSrv, logger, stopAdmission)

	runErr, drainErr := runAndJoinOriginalDrain(ctx, a)
	if drainErr != nil {
		logger.Error("original adapter drain is unqualified; holding process", zap.Error(drainErr))
		for {
			time.Sleep(time.Hour)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, shutdown := range []func(context.Context) error{whSrv.Shutdown, healthSrv.Shutdown} {
		if err := shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Warn("side listener shutdown exceeded wait; joining original work", zap.Error(err))
			if joined := shutdown(context.Background()); joined != nil {
				logger.Error("original side listener completion is unqualified; holding process", zap.Error(joined))
				for {
					time.Sleep(time.Hour)
				}
			}
		}
	}
	if runErr != nil {
		logger.Fatal("adapter run after original drain joined", zap.Error(runErr))
	}
	for _, failure := range []<-chan error{webhookFailure, healthFailure} {
		select {
		case err := <-failure:
			logger.Fatal("side listener after original drain joined", zap.Error(err))
		default:
		}
	}
}

// A failed side listener stops the same Run's admission, then its original
// completion still owns all provider handlers, replies and settlement.
func serveHealthServer(server *http.Server, logger *zap.Logger, stopAdmission context.CancelFunc) <-chan error {
	return serveSideListener(server.ListenAndServe, logger, stopAdmission)
}

func serveSideListener(serve func() error, logger *zap.Logger, stopAdmission context.CancelFunc) <-chan error {
	failure := make(chan error, 1)
	go func() {
		if err := serve(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("side listener failed; stopping admission", zap.Error(err))
			failure <- err
			stopAdmission()
		}
	}()
	return failure
}

// originalDrainRunner is the same SDK runtime that owns the first Run handle.
// WaitDrained limits no handler and never starts replacement cleanup.
type originalDrainRunner interface {
	Run(context.Context) error
	WaitDrained(context.Context) error
}

func runAndJoinOriginalDrain(ctx context.Context, a originalDrainRunner) (error, error) {
	runErr := a.Run(ctx)
	return runErr, a.WaitDrained(context.Background())
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
	mux.Handle("/metrics", promhttp.Handler())
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

// defaultInstanceID picks a stable fallback InstanceID for the
// reconciler registration. Production routing per-request uses the
// envelope's integration.instance_id (lifted by the ExecuteHandler
// bridge); this fallback exists only for paths that omit the field
// (unit tests, smoke probes). Picks the lexicographically-first
// configured instance, or empty when none are configured.
func defaultInstanceID(instances map[string]config.InstanceConfig) string {
	if len(instances) == 0 {
		return ""
	}
	first := ""
	for id := range instances {
		if first == "" || id < first {
			first = id
		}
	}
	return first
}
