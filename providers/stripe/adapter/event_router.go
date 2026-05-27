package adapter

// eventTypeToRTAKey is the canonical Stripe-event-type → RTA-routing-key
// mapping (spec §7). All 18 documented event types route to a specific
// key; anything else falls into the catch-all "rta.stripe.unhandled_event".
func eventTypeToRTAKey(eventType string) string {
	switch eventType {
	case "payment_intent.succeeded":
		return "rta.payments.intent_succeeded"
	case "payment_intent.payment_failed":
		return "rta.payments.intent_failed"
	case "payment_intent.canceled":
		return "rta.payments.intent_canceled"
	case "payment_intent.requires_action":
		return "rta.payments.intent_requires_action"
	case "charge.refunded":
		return "rta.payments.refunded"
	case "charge.dispute.created":
		return "rta.payments.dispute_created"
	case "charge.dispute.closed":
		return "rta.payments.dispute_closed"
	case "invoice.paid":
		return "rta.payments.invoice_paid"
	case "invoice.payment_failed":
		return "rta.payments.invoice_failed"
	case "balance.available":
		return "rta.payments.balance_available"
	case "payout.paid":
		return "rta.payments.payout_paid"
	case "payout.failed":
		return "rta.payments.payout_failed"
	case "payout.reconciliation_completed":
		return "rta.payments.payout_reconciliation_completed"
	case "customer.subscription.deleted":
		return "rta.subscriptions.cancelled"
	case "customer.subscription.updated":
		return "rta.subscriptions.updated"
	case "customer.subscription.trial_will_end":
		return "rta.subscriptions.trial_ending"
	case "account.updated":
		return "rta.connect.account_updated"
	case "account.application.deauthorized":
		return "rta.connect.deauthorized"
	default:
		return "rta.stripe.unhandled_event"
	}
}
