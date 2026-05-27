package message

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dakasa-yggdrasil/yggdrasil-sdk-go/rpc"
	"go.uber.org/zap"

	"github.com/dakasa-yggdrasil/integration-stripe/providers/stripe/config"
)

// ExecuteHandler returns an SDK-shaped handler for the execute
// capability. The Task 7 stub returns not_implemented; Task 20 onwards
// expands this to dispatch through adapter.Execute.
func ExecuteHandler(logger *zap.Logger, instances map[string]config.InstanceConfig) Handler {
	_ = instances // silenced until Task 20 wires per-instance routing
	return func(ctx context.Context, d rpc.Delivery) ([]byte, string, error) {
		var envelope struct {
			Operation  string `json:"operation"`
			Capability string `json:"capability,omitempty"`
		}
		if err := json.Unmarshal(d.Body, &envelope); err != nil {
			return failure("bad_request", err, logger)
		}
		body, _ := json.Marshal(map[string]string{
			"status":  "not_implemented",
			"message": fmt.Sprintf("operation %q not yet wired", envelope.Operation),
		})
		return body, "application/json", nil
	}
}
