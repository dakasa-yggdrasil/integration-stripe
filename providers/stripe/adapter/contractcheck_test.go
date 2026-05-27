package adapter

import (
	"testing"

	"github.com/dakasa-yggdrasil/integration-stripe/pkg/contractcheck"
)

// TestContractCheck verifies that every entry of SupportedExecuteOperations
// appears in Describe().ActionCatalog with the right resource_types, and
// that no execute-catalog entry has drifted out of SupportedExecuteOperations.
// Reactor entries (Category="reactor") are excluded from the lint because
// they are framework-invoked, not executable via execute.
//
// Drift here is caught by the dedicated CI job in .github/workflows/ci.yml.
func TestContractCheck(t *testing.T) {
	desc := Describe()

	// Filter out reactor entries — contractcheck enforces a strict
	// mapping with SupportedExecuteOperations, but reactors are
	// intentionally absent from that list.
	executeCatalog := make([]contractcheck.ActionDefinition, 0, len(desc.ActionCatalog))
	for _, action := range desc.ActionCatalog {
		if action.Category == "reactor" {
			continue
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
