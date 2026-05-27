package message

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dakasa-yggdrasil/yggdrasil-sdk-go/rpc"
	"go.uber.org/zap"

	ad "github.com/dakasa-yggdrasil/integration-stripe/providers/stripe/adapter"
	"github.com/dakasa-yggdrasil/integration-stripe/providers/stripe/config"
	model "github.com/dakasa-yggdrasil/integration-stripe/family/contract"
)

// ExecuteHandler returns an SDK-shaped handler for the execute
// capability. Dispatches through adapter.Execute, which owns the per-
// capability switch and the per-instance client lookup.
func ExecuteHandler(logger *zap.Logger, instances map[string]config.InstanceConfig) Handler {
	_ = instances // per-instance config consumed inside adapter.Execute via clientForInstance
	return func(ctx context.Context, d rpc.Delivery) ([]byte, string, error) {
		var envelope struct {
			Operation  string `json:"operation"`
			Capability string `json:"capability,omitempty"`
		}
		if err := json.Unmarshal(d.Body, &envelope); err != nil {
			return failure("bad_request", err, logger)
		}

		capability := envelope.Capability
		if capability == "" {
			capability = envelope.Operation
		}
		if !ad.SupportsExecuteCapability(capability) {
			return failure("unsupported_capability",
				fmt.Errorf("unsupported capability %q", capability), logger)
		}

		var req model.AdapterExecuteIntegrationRequest
		if err := json.Unmarshal(d.Body, &req); err != nil {
			return failure("bad_request", err, logger)
		}
		response, err := ad.Execute(req)
		if err != nil {
			return failure("execute_failed", err, logger)
		}
		return success(response)
	}
}
