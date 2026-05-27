package adapter

import (
	"context"
	"fmt"
	"strings"

	"github.com/stripe/stripe-go/v83"

	"github.com/dakasa-yggdrasil/integration-stripe/family/contract"
)

// manageConnectAccount implements the manage_connect_account capability
// (spec §3.12). Phase 1 supports create / get / update. Anything else
// returns unsupported_operation so workflow authors get a clear error
// instead of a silent no-op.
func manageConnectAccount(ctx context.Context, c *stripe.Client, req contract.AdapterExecuteIntegrationRequest) (contract.AdapterExecuteIntegrationResponse, error) {
	op := strings.ToLower(strings.TrimSpace(stringOr(req.Input, "operation")))
	switch op {
	case "create":
		return connectCreate(ctx, c, req)
	case "get":
		return connectGet(ctx, c, req)
	case "update":
		return connectUpdate(ctx, c, req)
	default:
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("unsupported_operation: %q (Phase 1 supports create|get|update)", op)
	}
}

func connectCreate(ctx context.Context, c *stripe.Client, req contract.AdapterExecuteIntegrationRequest) (contract.AdapterExecuteIntegrationResponse, error) {
	email := stringOr(req.Input, "email")
	country := stringOr(req.Input, "country")
	if country == "" {
		country = "BR"
	}
	acctType := stringOr(req.Input, "type")
	if acctType == "" {
		acctType = "express"
	}
	params := &stripe.AccountCreateParams{
		Type:    stripe.String(acctType),
		Country: stripe.String(country),
	}
	if email != "" {
		params.Email = stripe.String(email)
	}
	params.SetIdempotencyKey(idempotencyKeyOrDerived("", "manage_acct_create", email, country))

	acct, err := c.V1Accounts.Create(ctx, params)
	if err != nil {
		return contract.AdapterExecuteIntegrationResponse{}, err
	}
	return contract.AdapterExecuteIntegrationResponse{Output: acctOutput(acct)}, nil
}

func connectGet(ctx context.Context, c *stripe.Client, req contract.AdapterExecuteIntegrationRequest) (contract.AdapterExecuteIntegrationResponse, error) {
	id := stringOr(req.Input, "account_id")
	if id == "" {
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("account_id required for get")
	}
	acct, err := c.V1Accounts.GetByID(ctx, id, nil)
	if err != nil {
		return contract.AdapterExecuteIntegrationResponse{}, err
	}
	return contract.AdapterExecuteIntegrationResponse{Output: acctOutput(acct)}, nil
}

func connectUpdate(ctx context.Context, c *stripe.Client, req contract.AdapterExecuteIntegrationRequest) (contract.AdapterExecuteIntegrationResponse, error) {
	id := stringOr(req.Input, "account_id")
	if id == "" {
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("account_id required for update")
	}
	params := &stripe.AccountUpdateParams{}
	if email := stringOr(req.Input, "email"); email != "" {
		params.Email = stripe.String(email)
	}
	acct, err := c.V1Accounts.Update(ctx, id, params)
	if err != nil {
		return contract.AdapterExecuteIntegrationResponse{}, err
	}
	return contract.AdapterExecuteIntegrationResponse{Output: acctOutput(acct)}, nil
}

func acctOutput(a *stripe.Account) map[string]any {
	return map[string]any{
		"account_id":        a.ID,
		"type":              string(a.Type),
		"country":           a.Country,
		"charges_enabled":   a.ChargesEnabled,
		"payouts_enabled":   a.PayoutsEnabled,
		"details_submitted": a.DetailsSubmitted,
	}
}
