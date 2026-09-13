package adapter

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dakasa-yggdrasil/integration-stripe/family/contract"
	"github.com/stretchr/testify/require"
)

func TestDestroyWebhookEndpointAuthoritativeAbsence(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		absent bool
	}{
		{"missing", 404, true}, {"forbidden", 403, false}, {"unauthorized", 401, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodDelete, r.Method)
				require.Equal(t, "/v1/webhook_endpoints/we_gone", r.URL.Path)
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, `{"error":{"type":"invalid_request_error","code":"resource_missing","message":"gone"}}`)
			}))
			defer ts.Close()
			client, err := NewStripeClient("sk_test", ts.URL, StripeAPIVersion)
			require.NoError(t, err)
			resp, err := destroyWebhookEndpoint(context.Background(), client, contract.AdapterExecuteIntegrationRequest{Input: map[string]any{"ref": "we_gone"}})
			if tc.absent {
				require.NoError(t, err)
				require.Equal(t, map[string]any{"id": "we_gone", "deleted": true, "already_absent": true}, resp.Output)
			} else {
				require.Error(t, err)
				require.Empty(t, resp.Output)
			}
		})
	}
}

func TestDestroyWebhookEndpointRequiresIDOrRef(t *testing.T) {
	_, err := destroyWebhookEndpoint(context.Background(), nil, contract.AdapterExecuteIntegrationRequest{})
	require.ErrorContains(t, err, "id (or ref) required")
}
