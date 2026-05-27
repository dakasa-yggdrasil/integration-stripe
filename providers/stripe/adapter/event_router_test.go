package adapter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEventTypeToRTAKey_AllEighteen(t *testing.T) {
	want := map[string]string{
		"payment_intent.succeeded":             "rta.payments.intent_succeeded",
		"payment_intent.payment_failed":        "rta.payments.intent_failed",
		"payment_intent.canceled":              "rta.payments.intent_canceled",
		"payment_intent.requires_action":       "rta.payments.intent_requires_action",
		"charge.refunded":                      "rta.payments.refunded",
		"charge.dispute.created":               "rta.payments.dispute_created",
		"charge.dispute.closed":                "rta.payments.dispute_closed",
		"invoice.paid":                         "rta.payments.invoice_paid",
		"invoice.payment_failed":               "rta.payments.invoice_failed",
		"balance.available":                    "rta.payments.balance_available",
		"payout.paid":                          "rta.payments.payout_paid",
		"payout.failed":                        "rta.payments.payout_failed",
		"payout.reconciliation_completed":      "rta.payments.payout_reconciliation_completed",
		"customer.subscription.deleted":        "rta.subscriptions.cancelled",
		"customer.subscription.updated":        "rta.subscriptions.updated",
		"customer.subscription.trial_will_end": "rta.subscriptions.trial_ending",
		"account.updated":                      "rta.connect.account_updated",
		"account.application.deauthorized":     "rta.connect.deauthorized",
	}
	require.Len(t, want, 18)
	for et, rk := range want {
		require.Equal(t, rk, eventTypeToRTAKey(et), "event %q", et)
	}
}

func TestEventTypeToRTAKey_CatchAll(t *testing.T) {
	require.Equal(t, "rta.stripe.unhandled_event", eventTypeToRTAKey("foo.bar.unknown"))
	require.Equal(t, "rta.stripe.unhandled_event", eventTypeToRTAKey(""))
}
