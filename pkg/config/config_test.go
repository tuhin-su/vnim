package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigValidation(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "vnim-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	validYAML := `
objects:
  - type: bridge
    name: br0
  - type: tap
    name: tap1
    owner: ${USER}
    master: br0
  - type: address
    interface: br0
    address: 192.168.100.1/24
services:
  - type: dhcp
    interface: br0
    subnet: 192.168.100.0/24
    range_start: 192.168.100.10
    range_end: 192.168.100.50
`
	yamlPath := filepath.Join(tempDir, "lab.yaml")
	if err := os.WriteFile(yamlPath, []byte(validYAML), 0644); err != nil {
		t.Fatalf("failed to write test yaml: %v", err)
	}

	cfg, err := LoadConfig(yamlPath)
	if err != nil {
		t.Fatalf("failed to load valid config: %v", err)
	}

	if err := cfg.GetErrors(); err != nil {
		t.Errorf("expected no validation errors, got: %v", err)
	}

	// Test invalid YAML validation
	invalidYAML := `
objects:
  - type: invalid_type
    name: bad_name
  - type: tap
    name: interfaceNameTooLongBeholdThisNameIsVeryLongIndeed
  - type: veth
    name: veth0
`
	invalidPath := filepath.Join(tempDir, "invalid.yaml")
	if err := os.WriteFile(invalidPath, []byte(invalidYAML), 0644); err != nil {
		t.Fatalf("failed to write test yaml: %v", err)
	}

	cfg2, err := LoadConfig(invalidPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	errs := cfg2.Validate()
	if len(errs) == 0 {
		t.Errorf("expected validation errors, got none")
	}

	expectedErrors := []string{
		"invalid type",
		"exceeds Linux limit",
		"requires a peer interface name",
	}

	for _, expected := range expectedErrors {
		found := false
		for _, err := range errs {
			if contains(err.Error(), expected) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected error containing %q, but was not found in: %v", expected, errs)
		}
	}
}

func contains(str, substr string) bool {
	return filepath.Clean(str) != "" && (len(substr) == 0 || len(str) >= len(substr) && (str[:len(substr)] == substr || contains(str[1:], substr)))
}

func TestPluginConfigValidation(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "vnim-plugin-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	validPluginYAML := `
objects:
  - type: bridge
    name: br0
services:
  - type: plugin
    interface: br0
    mode: exec
    script: echo "hello"
`
	validPath := filepath.Join(tempDir, "plugin_valid.yaml")
	_ = os.WriteFile(validPath, []byte(validPluginYAML), 0644)
	cfg, err := LoadConfig(validPath)
	if err != nil {
		t.Fatalf("failed to load valid plugin config: %v", err)
	}
	if err := cfg.GetErrors(); err != nil {
		t.Errorf("expected no validation errors, got: %v", err)
	}

	invalidPluginYAML := `
objects:
  - type: bridge
    name: br0
services:
  - type: plugin
    interface: br0
    mode: invalid-mode
    script: echo "hello"
    path: /usr/bin/echo
`
	invalidPath := filepath.Join(tempDir, "plugin_invalid.yaml")
	_ = os.WriteFile(invalidPath, []byte(invalidPluginYAML), 0644)
	cfg2, err := LoadConfig(invalidPath)
	if err != nil {
		t.Fatalf("failed to load invalid plugin config: %v", err)
	}
	errs := cfg2.Validate()
	if len(errs) != 2 {
		t.Errorf("expected 2 validation errors, got %d: %v", len(errs), errs)
	}
}
