package message

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dakasa-yggdrasil/yggdrasil-sdk-go/adapter"
	"github.com/dakasa-yggdrasil/yggdrasil-sdk-go/rpc"
	"github.com/dakasa-yggdrasil/yggdrasil-sdk-go/sdk/reconcile"
	"go.uber.org/zap"

	ad "github.com/dakasa-yggdrasil/integration-stripe/providers/stripe/adapter"
	"github.com/dakasa-yggdrasil/integration-stripe/providers/stripe/config"
	model "github.com/dakasa-yggdrasil/integration-stripe/family/contract"
)

// ExecuteHandler returns an SDK-shaped handler for the execute
// capability. Production wiring (v2.2.0+): routes inbound envelopes
// through reconcile.Dispatch first — activating §6.5 mutation event
// auto-emission via the WireReconcilers-installed dispatch table.
// Operations not registered with a Reconciler (allowlisted action
// helpers, verify_webhook_signature, manage_connect_account) fall
// back to the legacy adapter.Execute switch path.
func ExecuteHandler(logger *zap.Logger, a *adapter.Adapter, instances map[string]config.InstanceConfig) Handler {
	_ = instances // per-instance config consumed inside adapter.Execute via clientForInstance
	return func(ctx context.Context, d rpc.Delivery) ([]byte, string, error) {
		var req model.AdapterExecuteIntegrationRequest
		if err := json.Unmarshal(d.Body, &req); err != nil {
			return failure("bad_request", err, logger)
		}

		capability := req.Capability
		if capability == "" {
			capability = req.Operation
		}
		if !ad.SupportsExecuteCapability(capability) {
			return failure("unsupported_capability",
				fmt.Errorf("unsupported capability %q", capability), logger)
		}

		// Bridge: rebuild the SDK-shaped envelope so reconcile.Dispatch
		// can route by operation. The SDK's executeRequest reads
		// {operation, capability, instance_id, input} at the top level
		// — the wire-level integration.instance_id MUST be lifted so
		// §6.5 emission metadata + reconciler-side instance lookup work.
		sdkDelivery, sdkErr := buildSDKDelivery(d, req)
		if sdkErr != nil {
			return failure("bad_request", sdkErr, logger)
		}
		body, _, dispatchErr := reconcile.Dispatch(ctx, a, sdkDelivery)
		if dispatchErr == nil {
			// SDK reconcile path succeeded — re-wrap the raw observed
			// JSON in the adapter's rpcResponse envelope so callers see
			// the same {ok,data} shape they always have.
			var out map[string]any
			if err := json.Unmarshal(body, &out); err != nil {
				// Body was non-object (e.g. observe items wrapped).
				out = map[string]any{"raw": json.RawMessage(body)}
			}
			return success(model.AdapterExecuteIntegrationResponse{
				Operation:  req.Operation,
				Capability: req.Capability,
				Status:     "ok",
				Output:     out,
			})
		}
		if !isUnsupportedReconcileOp(dispatchErr) {
			return failure("execute_failed", dispatchErr, logger)
		}

		// Operation has no Reconciler — fall back to the legacy switch
		// (action helpers, verify_webhook_signature, etc.).
		response, err := ad.Execute(req)
		if err != nil {
			return failure("execute_failed", err, logger)
		}
		return success(response)
	}
}

// buildSDKDelivery rewrites the inbound wire body into the shape the
// SDK reconcile dispatch expects: {operation, capability, instance_id,
// idempotency, input}. The instance_id is lifted from
// integration.instance_id AND injected into input.instance_id so the
// in-tree reconciler dispatch helpers can extract it per-call (the
// SDK doesn't forward env.InstanceID into Reconciler.Ensure — only
// onto the auto-emitted MutationEvent).
func buildSDKDelivery(d rpc.Delivery, req model.AdapterExecuteIntegrationRequest) (rpc.Delivery, error) {
	input := req.Input
	if input == nil {
		input = map[string]any{}
	}
	if strings.TrimSpace(req.Integration.InstanceID) != "" {
		if _, present := input["instance_id"]; !present {
			input["instance_id"] = req.Integration.InstanceID
		}
	}
	idempotency, _ := req.Metadata["idempotency"].(string)
	sdkBody, err := json.Marshal(map[string]any{
		"operation":   req.Operation,
		"capability":  req.Capability,
		"instance_id": req.Integration.InstanceID,
		"idempotency": idempotency,
		"input":       input,
	})
	if err != nil {
		return rpc.Delivery{}, err
	}
	return rpc.Delivery{Body: sdkBody, ContentType: d.ContentType}, nil
}

// isUnsupportedReconcileOp matches the SDK's "unsupported operation"
// signal so the bridge falls back to the legacy switch instead of
// surfacing the error to callers.
func isUnsupportedReconcileOp(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "reconcile: unsupported operation") ||
		strings.Contains(msg, "reconcile: adapter has no registered Reconciler")
}
