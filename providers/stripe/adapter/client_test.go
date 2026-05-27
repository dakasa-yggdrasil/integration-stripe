package adapter

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v83"
)

func TestNewStripeClient_PerInstanceKey_Isolated(t *testing.T) {
	var mu sync.Mutex
	keys := []string{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		keys = append(keys, strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		mu.Unlock()
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = w.Write([]byte(`{"id":"cus_x","object":"customer","email":"a@b.com","created":1700000000}`))
	}))
	defer ts.Close()

	clientA, err := NewStripeClient("sk_test_AAA", ts.URL, StripeAPIVersion)
	require.NoError(t, err)
	clientB, err := NewStripeClient("sk_test_BBB", ts.URL, StripeAPIVersion)
	require.NoError(t, err)

	_, err = clientA.V1Customers.Create(context.Background(), &stripe.CustomerCreateParams{
		Email: stripe.String("a@a.com"),
	})
	require.NoError(t, err)

	_, err = clientB.V1Customers.Create(context.Background(), &stripe.CustomerCreateParams{
		Email: stripe.String("b@b.com"),
	})
	require.NoError(t, err)

	// Two distinct keys reached the fake Stripe — proves per-instance
	// isolation (no global singleton bleed).
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, keys, 2)
	require.NotEqual(t, keys[0], keys[1], "each instance must send its own API key")
}

func TestIdempotencyKey_DerivedFromInputOrSha256Stable(t *testing.T) {
	// Provided wins over derivation.
	require.Equal(t, "user_supplied",
		idempotencyKeyOrDerived("user_supplied", "create_pi", "1990", "brl"))

	// Same input → same derived key.
	k1 := idempotencyKeyOrDerived("", "create_pi", "1990", "brl")
	k2 := idempotencyKeyOrDerived("", "create_pi", "1990", "brl")
	require.Equal(t, k1, k2, "derivation must be stable")

	// Different input → different key.
	k3 := idempotencyKeyOrDerived("", "create_pi", "1991", "brl")
	require.NotEqual(t, k1, k3)

	// Operation prefix is present.
	require.True(t, strings.HasPrefix(k1, "create_pi_"))
}
