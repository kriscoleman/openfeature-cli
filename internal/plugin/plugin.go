// Package plugin provides the plugin architecture for sync providers
package plugin

import (
	"context"
	"fmt"

	"github.com/open-feature/cli/internal/flagset"
)

// Capability represents what operations a plugin supports
type Capability string

const (
	// CapabilityPull indicates the plugin can pull flags from a remote source
	CapabilityPull Capability = "pull"
	// CapabilityPush indicates the plugin can push flags to a remote source
	CapabilityPush Capability = "push"
	// CapabilityCompare indicates the plugin can compare local and remote flags
	CapabilityCompare Capability = "compare"
	// CapabilityDelete indicates the plugin can delete/archive flags remotely
	CapabilityDelete Capability = "delete"
)

// Stability indicates the maturity level of a plugin
type Stability string

const (
	StabilityExperimental Stability = "experimental"
	StabilityBeta         Stability = "beta"
	StabilityStable       Stability = "stable"
)

// Metadata contains information about a plugin
type Metadata struct {
	// Name is the unique identifier for the plugin (e.g., "default", "devcycle")
	Name string
	// Version is the semantic version of the plugin
	Version string
	// Description provides a brief explanation of the plugin
	Description string
	// Stability indicates the maturity level
	Stability Stability
	// Capabilities lists what operations the plugin supports
	Capabilities []Capability
	// ConfigSchema describes the configuration options (optional)
	ConfigSchema *ConfigSchema
}

// ConfigSchema describes the configuration options for a plugin
type ConfigSchema struct {
	// Required fields that must be provided
	Required []string
	// Properties describes each configuration field
	Properties map[string]ConfigProperty
}

// ConfigProperty describes a single configuration property
type ConfigProperty struct {
	Type        string // "string", "boolean", "integer", etc.
	Description string
	Default     any
	EnvVar      string // Environment variable to read from
	Sensitive   bool   // If true, the value should be masked in logs
}

// Config holds the configuration passed to a plugin
type Config struct {
	// BaseURL is the provider URL for sync operations
	BaseURL string
	// AuthToken is the authentication token
	AuthToken string
	// Custom holds provider-specific configuration
	Custom map[string]any
}

// PullOptions contains options for pull operations
type PullOptions struct {
	// Context for cancellation and timeouts
	Context context.Context
}

// PushOptions contains options for push operations
type PushOptions struct {
	// Context for cancellation and timeouts
	Context context.Context
	// DryRun if true, only simulates the push without making changes
	DryRun bool
}

// CompareOptions contains options for compare operations
type CompareOptions struct {
	// Context for cancellation and timeouts
	Context context.Context
}

// PushResult contains the results of a push operation
type PushResult struct {
	// Created contains flags that were newly created
	Created []flagset.Flag
	// Updated contains flags that were modified
	Updated []flagset.Flag
	// Deleted contains flags that were removed/archived
	Deleted []flagset.Flag
	// Unchanged contains flags that did not change
	Unchanged []flagset.Flag
	// Errors contains any non-fatal errors encountered
	Errors []error
}

// CompareResult contains the results of a compare operation
type CompareResult struct {
	// Added contains flags that exist locally but not remotely
	Added []flagset.Flag
	// Removed contains flags that exist remotely but not locally
	Removed []flagset.Flag
	// Modified contains flags that differ between local and remote
	Modified []FlagDiff
	// Unchanged contains flags that are identical
	Unchanged []flagset.Flag
}

// FlagDiff represents a difference between local and remote flag states
type FlagDiff struct {
	Key    string
	Local  flagset.Flag
	Remote flagset.Flag
}

// SyncPlugin defines the interface that all sync plugins must implement
type SyncPlugin interface {
	// Metadata returns information about the plugin
	Metadata() Metadata

	// Configure initializes the plugin with the provided configuration
	Configure(config Config) error

	// ValidateConfig validates the current configuration
	ValidateConfig() error

	// Pull fetches flags from the remote source
	// Returns an error if the plugin doesn't support pull operations
	Pull(opts PullOptions) (*flagset.Flagset, error)

	// Push sends flags to the remote source
	// Returns an error if the plugin doesn't support push operations
	Push(local *flagset.Flagset, opts PushOptions) (*PushResult, error)

	// Compare compares local flags with remote flags
	// Returns an error if the plugin doesn't support compare operations
	Compare(local *flagset.Flagset, opts CompareOptions) (*CompareResult, error)
}

// HasCapability checks if a plugin supports a given capability
func HasCapability(p SyncPlugin, cap Capability) bool {
	for _, c := range p.Metadata().Capabilities {
		if c == cap {
			return true
		}
	}
	return false
}

// ErrNotSupported is returned when an operation is not supported by a plugin
type ErrNotSupported struct {
	Plugin    string
	Operation string
}

func (e *ErrNotSupported) Error() string {
	return fmt.Sprintf("plugin %q does not support %s operation", e.Plugin, e.Operation)
}

// ErrConfigInvalid is returned when plugin configuration is invalid
type ErrConfigInvalid struct {
	Plugin  string
	Message string
}

func (e *ErrConfigInvalid) Error() string {
	return fmt.Sprintf("invalid configuration for plugin %q: %s", e.Plugin, e.Message)
}
