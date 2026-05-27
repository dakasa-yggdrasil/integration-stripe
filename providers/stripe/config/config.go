// Package config holds per-instance Stripe adapter configuration. Real
// content is added in Task 10; this stub keeps the package importable so
// cmd/adapter/main.go compiles.
package config

// InstanceConfig is filled out in Task 10.
type InstanceConfig struct{}

// LoadInstances is filled out in Task 10.
func LoadInstances(path string) (map[string]InstanceConfig, error) {
	return map[string]InstanceConfig{}, nil
}
