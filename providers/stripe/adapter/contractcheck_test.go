package adapter

import (
	"testing"

	"github.com/dakasa-yggdrasil/integration-stripe/pkg/contractcheck"
)

// TestContractCheck verifies that every entry of SupportedExecuteOperations
// appears in Describe().ActionCatalog with the right resource_types, and
// that no execute-catalog entry has drifted out of SupportedExecuteOperations.
//
// Reactor entries (Category="reactor") that are NOT in
// SupportedExecuteOperations are excluded from the lint because they are
// framework-invoked, not executable via execute (e.g. stripe_webhook_received).
// But a reactor that IS in SupportedExecuteOperations (on_surface_query — it
// dispatches through Execute by query_name yet is reactor-categorized so it
// hides from grant pickers) must stay in the linted catalog so the strict
// SupportedExecuteOperations ↔ ActionCatalog mapping holds.
//
// Drift here is caught by the dedicated CI job in .github/workflows/ci.yml.
func TestContractCheck(t *testing.T) {
	desc := Describe()

	supported := make(map[string]struct{}, len(SupportedExecuteOperations))
	for _, op := range SupportedExecuteOperations {
		supported[op] = struct{}{}
	}

	// Drop only reactor entries that aren't executable via execute. Keep
	// reactor-categorized ops that ARE in SupportedExecuteOperations so the
	// strict mapping the lint enforces still covers them.
	executeCatalog := make([]contractcheck.ActionDefinition, 0, len(desc.ActionCatalog))
	for _, action := range desc.ActionCatalog {
		if action.Category == "reactor" {
			if _, ok := supported[action.Name]; !ok {
				continue
			}
		}
		executeCatalog = append(executeCatalog, contractcheck.ActionDefinition{
			Name:          action.Name,
			ResourceTypes: action.ResourceTypes,
		})
	}

	resourceTypes := make([]contractcheck.ResourceType, 0, len(desc.ResourceTypes))
	for _, rt := range desc.ResourceTypes {
		resourceTypes = append(resourceTypes, contractcheck.ResourceType{
			Name:           rt.Name,
			DefaultActions: rt.DefaultActions,
		})
	}

	checkSpec := contractcheck.DescribeResponse{
		ResourceTypes: resourceTypes,
		ActionCatalog: executeCatalog,
		Execution: contractcheck.ExecutionSpec{
			IdempotentActions: desc.Execution.IdempotentActions,
		},
	}
	if err := contractcheck.LintDescribeContract(checkSpec, SupportedExecuteOperations); err != nil {
		t.Fatalf("contract drift: %v", err)
	}
}
