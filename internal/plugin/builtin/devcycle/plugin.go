// Package devcycle provides a sync plugin for DevCycle feature flag management
package devcycle

import (
	"context"
	"fmt"

	"github.com/open-feature/cli/internal/flagset"
	"github.com/open-feature/cli/internal/logger"
	"github.com/open-feature/cli/internal/plugin"
)

// Plugin implements the SyncPlugin interface for DevCycle
type Plugin struct {
	config      plugin.Config
	client      *Client
	project     string
	environment string
}

// NewPlugin creates a new instance of the DevCycle sync plugin
func NewPlugin() plugin.SyncPlugin {
	return &Plugin{}
}

// Metadata returns information about the DevCycle plugin
func (p *Plugin) Metadata() plugin.Metadata {
	return plugin.Metadata{
		Name:        "devcycle",
		Version:     "1.0.0",
		Description: "Sync plugin for DevCycle feature flag management platform",
		Stability:   plugin.StabilityBeta,
		Capabilities: []plugin.Capability{
			plugin.CapabilityPull,
			plugin.CapabilityPush,
			plugin.CapabilityCompare,
		},
		ConfigSchema: &plugin.ConfigSchema{
			Required: []string{"project", "clientId", "clientSecret"},
			Properties: map[string]plugin.ConfigProperty{
				"project": {
					Type:        "string",
					Description: "DevCycle project key",
				},
				"clientId": {
					Type:        "string",
					Description: "DevCycle OAuth client ID",
					EnvVar:      "DEVCYCLE_CLIENT_ID",
				},
				"clientSecret": {
					Type:        "string",
					Description: "DevCycle OAuth client secret",
					EnvVar:      "DEVCYCLE_CLIENT_SECRET",
					Sensitive:   true,
				},
				"environment": {
					Type:        "string",
					Description: "Target environment for sync operations",
					Default:     "development",
				},
			},
		},
	}
}

// Configure initializes the plugin with the provided configuration
func (p *Plugin) Configure(config plugin.Config) error {
	p.config = config

	// Extract DevCycle-specific configuration
	if project, ok := config.Custom["project"].(string); ok {
		p.project = project
	}

	if env, ok := config.Custom["environment"].(string); ok {
		p.environment = env
	} else {
		p.environment = "development" // default
	}

	// Get OAuth credentials
	clientID := ""
	clientSecret := ""

	if id, ok := config.Custom["clientId"].(string); ok {
		clientID = id
	}
	if secret, ok := config.Custom["clientSecret"].(string); ok {
		clientSecret = secret
	}

	// Create the DevCycle client
	client, err := NewClient(clientID, clientSecret)
	if err != nil {
		return fmt.Errorf("failed to create DevCycle client: %w", err)
	}
	p.client = client

	return nil
}

// ValidateConfig validates the current configuration
func (p *Plugin) ValidateConfig() error {
	if p.project == "" {
		return &plugin.ErrConfigInvalid{
			Plugin:  "devcycle",
			Message: "project is required",
		}
	}

	if p.client == nil {
		return &plugin.ErrConfigInvalid{
			Plugin:  "devcycle",
			Message: "client not configured - ensure clientId and clientSecret are provided",
		}
	}

	return nil
}

// Pull fetches flags from DevCycle
func (p *Plugin) Pull(opts plugin.PullOptions) (*flagset.Flagset, error) {
	if err := p.ValidateConfig(); err != nil {
		return nil, err
	}

	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}

	logger.Default.Debug(fmt.Sprintf("DevCyclePlugin: Pulling flags from project %s", p.project))

	// Fetch variables from DevCycle
	variables, err := p.client.GetVariables(ctx, p.project)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch DevCycle variables: %w", err)
	}

	// Convert DevCycle variables to OpenFeature flagset
	flags := make([]flagset.Flag, 0, len(variables))
	for _, v := range variables {
		flag, err := variableToFlag(v)
		if err != nil {
			logger.Default.Debug(fmt.Sprintf("DevCyclePlugin: Skipping variable %s: %v", v.Key, err))
			continue
		}
		flags = append(flags, flag)
	}

	logger.Default.Debug(fmt.Sprintf("DevCyclePlugin: Successfully pulled %d flags", len(flags)))

	return &flagset.Flagset{Flags: flags}, nil
}

// Push sends flags to DevCycle
func (p *Plugin) Push(local *flagset.Flagset, opts plugin.PushOptions) (*plugin.PushResult, error) {
	if err := p.ValidateConfig(); err != nil {
		return nil, err
	}

	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}

	logger.Default.Debug(fmt.Sprintf("DevCyclePlugin: Pushing %d flags to project %s (dry-run: %v)",
		len(local.Flags), p.project, opts.DryRun))

	// Fetch existing variables for comparison
	remoteVars, err := p.client.GetVariables(ctx, p.project)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch existing DevCycle variables: %w", err)
	}

	// Build map of existing variables
	remoteMap := make(map[string]Variable)
	for _, v := range remoteVars {
		remoteMap[v.Key] = v
	}

	result := &plugin.PushResult{}

	for _, flag := range local.Flags {
		variable := flagToVariable(flag, p.project)

		if existingVar, exists := remoteMap[flag.Key]; exists {
			// Check if update is needed
			if variableNeedsUpdate(existingVar, variable) {
				if !opts.DryRun {
					if err := p.client.UpdateVariable(ctx, p.project, flag.Key, variable); err != nil {
						result.Errors = append(result.Errors, fmt.Errorf("failed to update %s: %w", flag.Key, err))
						continue
					}
				}
				result.Updated = append(result.Updated, flag)
			} else {
				result.Unchanged = append(result.Unchanged, flag)
			}
		} else {
			// Create new variable (requires creating a feature first in DevCycle)
			if !opts.DryRun {
				if err := p.client.CreateFeatureWithVariable(ctx, p.project, variable); err != nil {
					result.Errors = append(result.Errors, fmt.Errorf("failed to create %s: %w", flag.Key, err))
					continue
				}
			}
			result.Created = append(result.Created, flag)
		}
	}

	return result, nil
}

// Compare compares local flags with DevCycle variables
func (p *Plugin) Compare(local *flagset.Flagset, opts plugin.CompareOptions) (*plugin.CompareResult, error) {
	if err := p.ValidateConfig(); err != nil {
		return nil, err
	}

	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}

	logger.Default.Debug(fmt.Sprintf("DevCyclePlugin: Comparing flags with project %s", p.project))

	// Fetch remote variables
	remoteVars, err := p.client.GetVariables(ctx, p.project)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch DevCycle variables: %w", err)
	}

	// Build maps for comparison
	localMap := make(map[string]flagset.Flag)
	for _, flag := range local.Flags {
		localMap[flag.Key] = flag
	}

	remoteMap := make(map[string]flagset.Flag)
	for _, v := range remoteVars {
		flag, err := variableToFlag(v)
		if err != nil {
			continue
		}
		remoteMap[flag.Key] = flag
	}

	result := &plugin.CompareResult{}

	// Find added and modified
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

	// Find removed
	for key, remoteFlag := range remoteMap {
		if _, exists := localMap[key]; !exists {
			result.Removed = append(result.Removed, remoteFlag)
		}
	}

	return result, nil
}

// variableToFlag converts a DevCycle variable to an OpenFeature flag
func variableToFlag(v Variable) (flagset.Flag, error) {
	flagType, err := devCycleTypeToFlagType(v.Type)
	if err != nil {
		return flagset.Flag{}, err
	}

	return flagset.Flag{
		Key:          v.Key,
		Type:         flagType,
		Description:  v.Description,
		DefaultValue: v.DefaultValue,
	}, nil
}

// flagToVariable converts an OpenFeature flag to a DevCycle variable
func flagToVariable(flag flagset.Flag, project string) Variable {
	return Variable{
		Key:          flag.Key,
		Type:         flagTypeToDevCycleType(flag.Type),
		Description:  flag.Description,
		DefaultValue: flag.DefaultValue,
	}
}

// devCycleTypeToFlagType converts DevCycle variable type to OpenFeature flag type
func devCycleTypeToFlagType(dcType string) (flagset.FlagType, error) {
	switch dcType {
	case "Boolean":
		return flagset.BoolType, nil
	case "String":
		return flagset.StringType, nil
	case "Number":
		return flagset.FloatType, nil
	case "JSON":
		return flagset.ObjectType, nil
	default:
		return flagset.UnknownFlagType, fmt.Errorf("unsupported DevCycle type: %s", dcType)
	}
}

// flagTypeToDevCycleType converts OpenFeature flag type to DevCycle variable type
func flagTypeToDevCycleType(ft flagset.FlagType) string {
	switch ft {
	case flagset.BoolType:
		return "Boolean"
	case flagset.StringType:
		return "String"
	case flagset.IntType, flagset.FloatType:
		return "Number"
	case flagset.ObjectType:
		return "JSON"
	default:
		return "String"
	}
}

// variableNeedsUpdate checks if a variable needs to be updated
func variableNeedsUpdate(existing, new Variable) bool {
	if existing.Type != new.Type {
		return true
	}
	if existing.Description != new.Description {
		return true
	}
	// Compare default values
	return fmt.Sprintf("%v", existing.DefaultValue) != fmt.Sprintf("%v", new.DefaultValue)
}

// flagsEqual compares two flags for equality
func flagsEqual(a, b flagset.Flag) bool {
	if a.Key != b.Key || a.Type != b.Type || a.Description != b.Description {
		return false
	}
	return fmt.Sprintf("%v", a.DefaultValue) == fmt.Sprintf("%v", b.DefaultValue)
}

func init() {
	// Register the DevCycle plugin with the default manager
	if err := plugin.Register(NewPlugin); err != nil {
		// This should never happen in normal operation
		panic(fmt.Sprintf("failed to register devcycle plugin: %v", err))
	}
}
