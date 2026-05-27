package message

import (
	"encoding/json"

	"github.com/dakasa-yggdrasil/yggdrasil-sdk-go/adapter"
	"go.uber.org/zap"
)

// Handler aliases adapter.Handler so the rest of this package can
// declare handler returns without importing the SDK everywhere.
type Handler = adapter.Handler

// rpcError is the shape of the error field inside rpcResponse.
type rpcError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// rpcResponse preserves the wire shape every dakasa-yggdrasil/integration-*
// adapter publishes: {ok, data?, error?}. The SDK handles transport-level
// framing (HTTP body, AMQP publishing) around this JSON.
type rpcResponse struct {
	OK    bool      `json:"ok"`
	Data  any       `json:"data,omitempty"`
	Error *rpcError `json:"error,omitempty"`
}

// success returns a successful JSON envelope for the SDK handler.
func success(data any) ([]byte, string, error) {
	body, err := json.Marshal(rpcResponse{OK: true, Data: data})
	if err != nil {
		return nil, "", err
	}
	return body, "application/json", nil
}

// failure returns a structured error envelope. The handler does NOT
// return a Go error because the adapter protocol expresses business
// errors as ok=false inside the body — returning err would Nack the
// SDK delivery and surface a generic transport error.
func failure(code string, cause error, logger *zap.Logger) ([]byte, string, error) {
	if logger != nil {
		logger.Error("adapter rpc handler failed",
			zap.String("error_code", code),
			zap.Error(cause),
		)
	}
	body, err := json.Marshal(rpcResponse{
		OK: false,
		Error: &rpcError{
			Code:    code,
			Message: cause.Error(),
		},
	})
	if err != nil {
		return nil, "", err
	}
	return body, "application/json", nil
}
