// Package contract defines the shared protocol types exchanged between
// yggdrasil-core and the stripe integration adapter. Mirrors the JSON
// wire shape used by every other dakasa-yggdrasil/integration-* repo;
// kept local to this module so the adapter stays standalone (no import
// of yggdrasil-core types).
package contract

import "github.com/google/uuid"

type ManifestSelector struct {
	ManifestID string `json:"manifest_id,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
	Name       string `json:"name,omitempty"`
	Version    *int   `json:"version,omitempty"`
}

type ManifestReference struct {
	ID        uuid.UUID `json:"id"`
	Kind      string    `json:"kind"`
	Namespace string    `json:"namespace"`
	Name      string    `json:"name"`
	Version   int       `json:"version"`
}

type IntegrationTypeManifestSpec struct {
	Provider              string                        `json:"provider"`
	FamilyRef             *ManifestSelector             `json:"family_ref,omitempty"`
	ImplementedOperations []string                      `json:"implemented_operations,omitempty"`
	Adapter               IntegrationAdapterSpec        `json:"adapter"`
	Capabilities          []string                      `json:"capabilities"`
	CredentialSchema      IntegrationSchemaSpec         `json:"credential_schema"`
	InstanceSchema        IntegrationSchemaSpec         `json:"instance_schema"`
	ResourceTypes         []IntegrationResourceType     `json:"resource_types"`
	ActionCatalog         []IntegrationActionDefinition `json:"action_catalog,omitempty"`
	Discovery             IntegrationDiscoverySpec      `json:"discovery"`
	Normalization         IntegrationNormalizationSpec  `json:"normalization"`
	Execution             IntegrationExecutionSpec      `json:"execution"`
	Extensions            IntegrationExtensionsSpec     `json:"extensions"`
}

type IntegrationAdapterSpec struct {
	Transport      string                  `json:"transport"`
	Version        string                  `json:"version"`
	Queues         IntegrationAdapterQueue `json:"queues,omitempty"`
	Endpoints      IntegrationAdapterRoute `json:"endpoints,omitempty"`
	TimeoutSeconds int                     `json:"timeout_seconds,omitempty"`
}

// IntegrationAdapterRoute carries the HTTP endpoint paths emitted in
// Describe() when the adapter is running with transport=http_json.
type IntegrationAdapterRoute struct {
	Describe string `json:"describe,omitempty"`
	Execute  string `json:"execute,omitempty"`
}

type IntegrationAdapterQueue struct {
	Describe string `json:"describe,omitempty"`
	Discover string `json:"discover,omitempty"`
	Read     string `json:"read,omitempty"`
	Execute  string `json:"execute,omitempty"`
	Sync     string `json:"sync,omitempty"`
	Health   string `json:"health,omitempty"`
}

type IntegrationSchemaSpec struct {
	Mode       string                               `json:"mode"`
	Required   []string                             `json:"required,omitempty"`
	Properties map[string]IntegrationSchemaProperty `json:"properties,omitempty"`
}

// IntegrationSchemaProperty is one field in a schema spec.
//
// §15 of INTEGRATION_CONTRACT.md (shipped 2026-05-27): adapter manifests
// MUST carry UI metadata so surfaces render generic forms WITHOUT per-provider
// hardcoded knowledge.
type IntegrationSchemaProperty struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Secret      bool   `json:"secret,omitempty"`
	Enum        []any  `json:"enum,omitempty"`
	Default     any    `json:"default,omitempty"`

	// UI metadata (§15) — drives generic form rendering in surfaces.
	Label             string                       `json:"label,omitempty"`
	LabelLocale       map[string]string            `json:"label_locale,omitempty"`
	Placeholder       string                       `json:"placeholder,omitempty"`
	PlaceholderLocale map[string]string            `json:"placeholder_locale,omitempty"`
	DescriptionLocale map[string]string            `json:"description_locale,omitempty"`
	Group             string                       `json:"group,omitempty"`
	GroupLocale       map[string]string            `json:"group_locale,omitempty"`
	Order             int                          `json:"order,omitempty"`
	Sensitive         bool                         `json:"sensitive,omitempty"`
	DependsOn         *IntegrationSchemaDependency `json:"depends_on,omitempty"`
	Format            string                       `json:"format,omitempty"`
}

// IntegrationSchemaDependency expresses a conditional-visibility relationship
// between two properties on the same schema (§15).
type IntegrationSchemaDependency struct {
	Field string `json:"field"`
	Value any    `json:"value"`
}

type IntegrationResourceType struct {
	Name             string   `json:"name"`
	CanonicalPrefix  string   `json:"canonical_prefix"`
	IdentityTemplate string   `json:"identity_template"`
	Discoverable     bool     `json:"discoverable"`
	DefaultActions   []string `json:"default_actions"`
}

type IntegrationActionDefinition struct {
	Name          string   `json:"name"`
	Description   string   `json:"description,omitempty"`
	ResourceTypes []string `json:"resource_types,omitempty"`
	Idempotent    bool     `json:"idempotent,omitempty"`
	Category      string   `json:"category,omitempty"`
}

type IntegrationDiscoverySpec struct {
	Mode             string `json:"mode"`
	Cursor           string `json:"cursor,omitempty"`
	SupportsWebhooks bool   `json:"supports_webhooks,omitempty"`
}

type IntegrationNormalizationSpec struct {
	ExternalIDPath         string `json:"external_id_path"`
	NamePath               string `json:"name_path,omitempty"`
	OwnerPath              string `json:"owner_path,omitempty"`
	FallbackResourcePrefix string `json:"fallback_resource_prefix"`
}

type IntegrationExecutionSpec struct {
	SupportsDryRun    bool     `json:"supports_dry_run,omitempty"`
	IdempotentActions []string `json:"idempotent_actions,omitempty"`
}

type IntegrationExtensionsSpec struct {
	AllowCustomResourceTypes bool `json:"allow_custom_resource_types,omitempty"`
	AllowCustomActions       bool `json:"allow_custom_actions,omitempty"`
	PreserveRawPayload       bool `json:"preserve_raw_payload,omitempty"`
}

type IntegrationInstanceManifestSpec struct {
	TypeRef     ManifestSelector                 `json:"type_ref"`
	Status      string                           `json:"status,omitempty"`
	Owners      []string                         `json:"owners,omitempty"`
	Credentials map[string]any                   `json:"credentials,omitempty"`
	Config      map[string]any                   `json:"config,omitempty"`
	Discovery   IntegrationInstanceDiscoverySpec `json:"discovery"`
	Execution   IntegrationInstanceExecutionSpec `json:"execution,omitempty"`
}

type IntegrationInstanceDiscoverySpec struct {
	Enabled             bool   `json:"enabled"`
	Mode                string `json:"mode,omitempty"`
	SyncIntervalSeconds int    `json:"sync_interval_seconds,omitempty"`
}

type IntegrationInstanceExecutionSpec struct {
	DefaultDryRun bool `json:"default_dry_run,omitempty"`
	MaxBatchSize  int  `json:"max_batch_size,omitempty"`
}

type AdapterDescribeRequest struct {
	Provider        string `json:"provider"`
	ExpectedVersion string `json:"expected_version,omitempty"`
}

type AdapterDescribeResponse struct {
	Provider         string                        `json:"provider"`
	Adapter          IntegrationAdapterSpec        `json:"adapter"`
	Capabilities     []string                      `json:"capabilities"`
	CredentialSchema IntegrationSchemaSpec         `json:"credential_schema"`
	InstanceSchema   IntegrationSchemaSpec         `json:"instance_schema"`
	ResourceTypes    []IntegrationResourceType     `json:"resource_types"`
	ActionCatalog    []IntegrationActionDefinition `json:"action_catalog,omitempty"`
	Discovery        IntegrationDiscoverySpec      `json:"discovery"`
	Normalization    IntegrationNormalizationSpec  `json:"normalization"`
	Execution        IntegrationExecutionSpec      `json:"execution"`
	Extensions       IntegrationExtensionsSpec     `json:"extensions"`
}

// IntegrationContext is the per-instance context attached to every
// AdapterExecuteIntegrationRequest. The stripe adapter routes by
// InstanceID for multi-tenant secret isolation.
type IntegrationContext struct {
	InstanceID string                          `json:"instance_id,omitempty"`
	Type       ManifestReference               `json:"type,omitempty"`
	TypeSpec   IntegrationTypeManifestSpec     `json:"type_spec,omitempty"`
	Instance   ManifestReference               `json:"instance,omitempty"`
	Spec       IntegrationInstanceManifestSpec `json:"instance_spec,omitempty"`
}

// AdapterExecuteIntegrationRequest is the input envelope for every
// execute capability dispatch.
type AdapterExecuteIntegrationRequest struct {
	Operation   string             `json:"operation"`
	Capability  string             `json:"capability,omitempty"`
	Input       map[string]any     `json:"input,omitempty"`
	Auth        map[string]any     `json:"auth,omitempty"`
	Metadata    map[string]any     `json:"metadata,omitempty"`
	Integration IntegrationContext `json:"integration"`
}

// AdapterExecuteIntegrationResponse is the output envelope returned by
// every execute capability.
type AdapterExecuteIntegrationResponse struct {
	Operation  string         `json:"operation,omitempty"`
	Capability string         `json:"capability,omitempty"`
	Status     string         `json:"status,omitempty"`
	Output     map[string]any `json:"output,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}
