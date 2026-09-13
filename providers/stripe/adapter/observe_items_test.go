package adapter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractItemsRequiresAnArrayOfObjects(t *testing.T) {
	for _, tc := range []struct {
		name     string
		response reconcilePayload
		valid    bool
		count    int
	}{
		{"native", reconcilePayload{"items": []map[string]any{{"id": "one"}}}, true, 1},
		{"json", reconcilePayload{"items": []any{map[string]any{"id": "one"}}}, true, 1},
		{"native_empty", reconcilePayload{"items": []map[string]any{}}, true, 0},
		{"json_empty", reconcilePayload{"items": []any{}}, true, 0},
		{"flat_regression", reconcilePayload{"id": "one"}, false, 0},
		{"nil_response", nil, false, 0},
		{"null", reconcilePayload{"items": nil}, false, 0},
		{"nil_native", reconcilePayload{"items": []map[string]any(nil)}, false, 0},
		{"nil_json", reconcilePayload{"items": []any(nil)}, false, 0},
		{"object", reconcilePayload{"items": map[string]any{"id": "one"}}, false, 0},
		{"null_native_item", reconcilePayload{"items": []map[string]any{nil}}, false, 0},
		{"null_json_item", reconcilePayload{"items": []any{nil}}, false, 0},
		{"scalar_item", reconcilePayload{"items": []any{map[string]any{"id": "one"}, "bad"}}, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			items, err := extractItems(tc.response)
			if tc.valid {
				require.NoError(t, err)
				require.NotNil(t, items)
				require.Len(t, items, tc.count)
			} else {
				require.ErrorContains(t, err, "observe response")
				require.Nil(t, items, "do not expose a partially decoded result")
			}
		})
	}
}

func TestObservedItemsSingleAndAbsentHaveNoMorePages(t *testing.T) {
	for _, output := range []map[string]any{observedItems().Output, observedItems(map[string]any{"id": "one"}).Output} {
		require.Equal(t, false, output["has_more"])
		require.NotNil(t, output["items"])
	}
}
