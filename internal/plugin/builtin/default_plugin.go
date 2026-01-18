// Package builtin provides built-in sync plugins that ship with the CLI
package builtin

import (
	"context"
	"fmt"

	"github.com/open-feature/cli/internal/api/sync"
	"github.com/open-feature/cli/internal/flagset"
	"github.com/open-feature/cli/internal/logger"
	"github.com/open-feature/cli/internal/plugin"
)

// DefaultPlugin implements the SyncPlugin interface using the standard OpenFeature
// Manifest Management API (api/v0/sync.yaml)
type DefaultPlugin struct {
	config plugin.Config
	client *sync.Client
}

// NewDefaultPlugin creates a new instance of the default sync plugin
func NewDefaultPlugin() plugin.SyncPlugin {
	return &DefaultPlugin{}
}

// Metadata returns information about the default plugin
func (p *DefaultPlugin) Metadata() plugin.Metadata {
	return plugin.Metadata{
		Name:        "default",
		Version:     "1.0.0",
		Description: "Default sync plugin using the OpenFeature Manifest Management API",
		Stability:   plugin.StabilityStable,
		Capabilities: []plugin.Capability{
			plugin.CapabilityPull,
			plugin.CapabilityPush,
			plugin.CapabilityCompare,
		},
		ConfigSchema: &plugin.ConfigSchema{
			Required: []string{},
			Properties: map[string]plugin.ConfigProperty{
				"baseUrl": {
					Type:        "string",
					Description: "Base URL of the Manifest Management API endpoint",
				},
				"authToken": {
					Type:        "string",
					Description: "Bearer token for API authentication",
					EnvVar:      "OPENFEATURE_AUTH_TOKEN",
					Sensitive:   true,
				},
			},
		},
	}
}

// Configure initializes the plugin with the provided configuration
func (p *DefaultPlugin) Configure(config plugin.Config) error {
	p.config = config

	// Create the sync client
	client, err := sync.NewClient(config.BaseURL, config.AuthToken)
	if err != nil {
		return fmt.Errorf("failed to create sync client: %w", err)
	}
	p.client = client

	return nil
}

// ValidateConfig validates the current configuration
func (p *DefaultPlugin) ValidateConfig() error {
	if p.config.BaseURL == "" {
		return &plugin.ErrConfigInvalid{
			Plugin:  "default",
			Message: "baseUrl is required",
		}
	}
	return nil
}

// Pull fetches flags from the remote source using the Manifest Management API
func (p *DefaultPlugin) Pull(opts plugin.PullOptions) (*flagset.Flagset, error) {
	if p.client == nil {
		return nil, fmt.Errorf("plugin not configured: call Configure() first")
	}

	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}

	logger.Default.Debug("DefaultPlugin: Pulling flags from remote source")
	return p.client.PullFlags(ctx)
}

// Push sends flags to the remote source using the Manifest Management API
func (p *DefaultPlugin) Push(local *flagset.Flagset, opts plugin.PushOptions) (*plugin.PushResult, error) {
	if p.client == nil {
		return nil, fmt.Errorf("plugin not configured: call Configure() first")
	}

	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}

	logger.Default.Debug("DefaultPlugin: Fetching remote flags for comparison")

	// First, pull remote flags for comparison
	remoteFlags, err := p.client.PullFlags(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch remote flags for comparison: %w", err)
	}

	logger.Default.Debug(fmt.Sprintf("DefaultPlugin: Pushing %d local flags (dry-run: %v)", len(local.Flags), opts.DryRun))

	// Perform the push using the sync client
	syncResult, err := p.client.PushFlags(ctx, local, remoteFlags, opts.DryRun)
	if err != nil {
		return nil, err
	}

	// Convert sync.PushResult to plugin.PushResult
	return &plugin.PushResult{
		Created:   syncResult.Created,
		Updated:   syncResult.Updated,
		Unchanged: syncResult.Unchanged,
	}, nil
}

// Compare compares local flags with remote flags
func (p *DefaultPlugin) Compare(local *flagset.Flagset, opts plugin.CompareOptions) (*plugin.CompareResult, error) {
	if p.client == nil {
		return nil, fmt.Errorf("plugin not configured: call Configure() first")
	}

	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}

	logger.Default.Debug("DefaultPlugin: Fetching remote flags for comparison")

	// Pull remote flags
	remoteFlags, err := p.client.PullFlags(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch remote flags: %w", err)
	}

	// Build maps for comparison
	localMap := make(map[string]flagset.Flag)
	for _, flag := range local.Flags {
		localMap[flag.Key] = flag
	}

	remoteMap := make(map[string]flagset.Flag)
	for _, flag := range remoteFlags.Flags {
		remoteMap[flag.Key] = flag
	}

	result := &plugin.CompareResult{}

	// Find added (local only) and modified flags
	for key, localFlag := range localMap {
		if remoteFlag, exists := remoteMap[key]; exists {
			if !flagsEqual(localFlag, remoteFlag) {
				result.Modified = append(result.Modified, plugin.FlagDiff{
					Key:    key,
					Local:  localFlag,
					Remote: remoteFlag,
				})
			} else {
				result.Unchanged = append(result.Unchanged, localFlag)
			}
		} else {
			result.Added = append(result.Added, localFlag)
		}
	}

	// Find removed (remote only) flags
	for key, remoteFlag := range remoteMap {
		if _, exists := localMap[key]; !exists {
			result.Removed = append(result.Removed, remoteFlag)
		}
	}

	return result, nil
}

// flagsEqual compares two flags for equality
func flagsEqual(a, b flagset.Flag) bool {
	if a.Key != b.Key || a.Type != b.Type || a.Description != b.Description {
		return false
	}

	// Compare default values - this is a simplified comparison
	// For more complex objects, we might need deep comparison
	return fmt.Sprintf("%v", a.DefaultValue) == fmt.Sprintf("%v", b.DefaultValue)
}

func init() {
	// Register the default plugin with the default manager
	if err := plugin.Register(NewDefaultPlugin); err != nil {
		// This should never happen in normal operation
		panic(fmt.Sprintf("failed to register default plugin: %v", err))
	}
}
