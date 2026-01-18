package plugin

import (
	"testing"

	"github.com/open-feature/cli/internal/flagset"
)

func TestMetadata(t *testing.T) {
	meta := Metadata{
		Name:        "test-plugin",
		Version:     "1.0.0",
		Description: "A test plugin",
		Stability:   StabilityBeta,
		Capabilities: []Capability{
			CapabilityPull,
			CapabilityPush,
		},
	}

	if meta.Name != "test-plugin" {
		t.Errorf("expected name 'test-plugin', got %q", meta.Name)
	}

	if meta.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got %q", meta.Version)
	}

	if len(meta.Capabilities) != 2 {
		t.Errorf("expected 2 capabilities, got %d", len(meta.Capabilities))
	}
}

func TestConfig(t *testing.T) {
	config := Config{
		BaseURL:   "https://api.example.com",
		AuthToken: "test-token",
		Custom: map[string]any{
			"project": "my-project",
		},
	}

	if config.BaseURL != "https://api.example.com" {
		t.Errorf("expected BaseURL 'https://api.example.com', got %q", config.BaseURL)
	}

	if config.AuthToken != "test-token" {
		t.Errorf("expected AuthToken 'test-token', got %q", config.AuthToken)
	}

	if project, ok := config.Custom["project"].(string); !ok || project != "my-project" {
		t.Errorf("expected project 'my-project', got %v", config.Custom["project"])
	}
}

func TestErrNotSupported(t *testing.T) {
	err := &ErrNotSupported{
		Plugin:    "test-plugin",
		Operation: "delete",
	}

	expected := `plugin "test-plugin" does not support delete operation`
	if err.Error() != expected {
		t.Errorf("expected error %q, got %q", expected, err.Error())
	}
}

func TestErrConfigInvalid(t *testing.T) {
	err := &ErrConfigInvalid{
		Plugin:  "test-plugin",
		Message: "missing required field",
	}

	expected := `invalid configuration for plugin "test-plugin": missing required field`
	if err.Error() != expected {
		t.Errorf("expected error %q, got %q", expected, err.Error())
	}
}

// MockPlugin is a test implementation of SyncPlugin
type MockPlugin struct {
	metadata     Metadata
	configured   bool
	configError  error
	pullResult   *flagset.Flagset
	pullError    error
	pushResult   *PushResult
	pushError    error
	compareResult *CompareResult
	compareError error
}

func NewMockPlugin() SyncPlugin {
	return &MockPlugin{
		metadata: Metadata{
			Name:        "mock",
			Version:     "1.0.0",
			Description: "A mock plugin for testing",
			Stability:   StabilityExperimental,
			Capabilities: []Capability{
				CapabilityPull,
				CapabilityPush,
				CapabilityCompare,
			},
		},
	}
}

func (p *MockPlugin) Metadata() Metadata {
	return p.metadata
}

func (p *MockPlugin) Configure(config Config) error {
	if p.configError != nil {
		return p.configError
	}
	p.configured = true
	return nil
}

func (p *MockPlugin) ValidateConfig() error {
	if !p.configured {
		return &ErrConfigInvalid{
			Plugin:  p.metadata.Name,
			Message: "plugin not configured",
		}
	}
	return nil
}

func (p *MockPlugin) Pull(opts PullOptions) (*flagset.Flagset, error) {
	if p.pullError != nil {
		return nil, p.pullError
	}
	if p.pullResult != nil {
		return p.pullResult, nil
	}
	return &flagset.Flagset{
		Flags: []flagset.Flag{
			{
				Key:          "test-flag",
				Type:         flagset.BoolType,
				Description:  "A test flag",
				DefaultValue: false,
			},
		},
	}, nil
}

func (p *MockPlugin) Push(local *flagset.Flagset, opts PushOptions) (*PushResult, error) {
	if p.pushError != nil {
		return nil, p.pushError
	}
	if p.pushResult != nil {
		return p.pushResult, nil
	}
	return &PushResult{
		Created: local.Flags,
	}, nil
}

func (p *MockPlugin) Compare(local *flagset.Flagset, opts CompareOptions) (*CompareResult, error) {
	if p.compareError != nil {
		return nil, p.compareError
	}
	if p.compareResult != nil {
		return p.compareResult, nil
	}
	return &CompareResult{
		Added: local.Flags,
	}, nil
}

func TestHasCapability(t *testing.T) {
	mock := NewMockPlugin()

	tests := []struct {
		cap      Capability
		expected bool
	}{
		{CapabilityPull, true},
		{CapabilityPush, true},
		{CapabilityCompare, true},
		{CapabilityDelete, false},
	}

	for _, test := range tests {
		result := HasCapability(mock, test.cap)
		if result != test.expected {
			t.Errorf("HasCapability(%s) = %v, expected %v", test.cap, result, test.expected)
		}
	}
}

func TestPushResult(t *testing.T) {
	result := &PushResult{
		Created: []flagset.Flag{
			{Key: "new-flag", Type: flagset.BoolType, DefaultValue: true},
		},
		Updated: []flagset.Flag{
			{Key: "updated-flag", Type: flagset.StringType, DefaultValue: "new-value"},
		},
		Unchanged: []flagset.Flag{
			{Key: "unchanged-flag", Type: flagset.IntType, DefaultValue: 42},
		},
	}

	if len(result.Created) != 1 {
		t.Errorf("expected 1 created flag, got %d", len(result.Created))
	}

	if len(result.Updated) != 1 {
		t.Errorf("expected 1 updated flag, got %d", len(result.Updated))
	}

	if len(result.Unchanged) != 1 {
		t.Errorf("expected 1 unchanged flag, got %d", len(result.Unchanged))
	}
}

func TestCompareResult(t *testing.T) {
	result := &CompareResult{
		Added: []flagset.Flag{
			{Key: "new-flag", Type: flagset.BoolType},
		},
		Removed: []flagset.Flag{
			{Key: "old-flag", Type: flagset.StringType},
		},
		Modified: []FlagDiff{
			{
				Key:    "changed-flag",
				Local:  flagset.Flag{Key: "changed-flag", Type: flagset.IntType, DefaultValue: 10},
				Remote: flagset.Flag{Key: "changed-flag", Type: flagset.IntType, DefaultValue: 5},
			},
		},
	}

	if len(result.Added) != 1 {
		t.Errorf("expected 1 added flag, got %d", len(result.Added))
	}

	if len(result.Removed) != 1 {
		t.Errorf("expected 1 removed flag, got %d", len(result.Removed))
	}

	if len(result.Modified) != 1 {
		t.Errorf("expected 1 modified flag, got %d", len(result.Modified))
	}

	if result.Modified[0].Key != "changed-flag" {
		t.Errorf("expected modified flag key 'changed-flag', got %q", result.Modified[0].Key)
	}
}
