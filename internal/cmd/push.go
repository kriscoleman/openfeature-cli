package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/open-feature/cli/internal/api/sync"
	"github.com/open-feature/cli/internal/config"
	"github.com/open-feature/cli/internal/flagset"
	"github.com/open-feature/cli/internal/logger"
	"github.com/open-feature/cli/internal/manifest"
	"github.com/open-feature/cli/internal/plugin"
	_ "github.com/open-feature/cli/internal/plugin/builtin" // Register built-in plugins
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// GetPushCmd returns the command for pushing flags to a remote source
func GetPushCmd() *cobra.Command {
	pushCmd := &cobra.Command{
		Use:   "push",
		Short: "Push flag configurations to a remote source",
		Long: `The push command syncs local flag configurations to a remote flag management service.

This command reads your local flag manifest and intelligently pushes it to a specified
remote destination. It performs a smart push by:

1. Fetching existing flags from the remote
2. Comparing local flags with remote flags
3. Creating new flags that don't exist remotely
4. Updating existing flags that have changed

This approach ensures idempotent operations and prevents conflicts.

The pushed data follows the Manifest Management API OpenAPI specification defined at:
api/v0/sync.yaml

The API uses individual flag endpoints:
- POST /openfeature/v0/manifest/flags - Creates new flags
- PUT /openfeature/v0/manifest/flags/{key} - Updates existing flags
- GET /openfeature/v0/manifest - Fetches existing flags for comparison

Remote services implementing this API should accept the flag data in the format
specified by the OpenFeature flag manifest schema.

Note: The file:// scheme is not supported for push operations.
For local file operations, use standard shell commands like cp or mv.`,
		Example: `  # Push flags to a remote HTTPS endpoint (smart push: creates and updates as needed)
  openfeature push --provider-url https://api.example.com --auth-token secret-token

  # Push flags to an HTTP endpoint (development)
  openfeature push --provider-url http://localhost:8080

  # Dry run to preview what would be sent
  openfeature push --provider-url https://api.example.com --dry-run`,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return initializeConfig(cmd, "push")
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get configuration values
			providerURL := config.GetFlagSourceURL(cmd)
			manifestPath := config.GetManifestPath(cmd)
			authToken := config.GetAuthToken(cmd)
			dryRun := config.GetDryRun(cmd)
			pluginName := config.GetPluginName(cmd)

			// Load the local manifest first (needed for both plugin and legacy paths)
			flags, err := manifest.LoadFlagSet(manifestPath)
			if err != nil {
				return fmt.Errorf("error loading manifest from %s: %w", manifestPath, err)
			}

			// If a plugin is specified, use the plugin system
			if pluginName != "" {
				return pushWithPlugin(cmd, pluginName, flags, dryRun)
			}

			// Otherwise, use the existing behavior (backward compatible)
			// Validate destination URL is provided
			if providerURL == "" {
				return fmt.Errorf("provider URL is required. Please provide --provider-url")
			}

			// Parse and validate URL
			parsedURL, err := url.Parse(providerURL)
			if err != nil {
				return fmt.Errorf("invalid source URL: %w", err)
			}

			// Handle URL schemes
			switch parsedURL.Scheme {
			case "file":
				return fmt.Errorf("file:// scheme is not supported for push. Use standard shell commands (cp, mv) for local file operations")
			case "http", "https":
				// Perform smart push (fetches remote, compares, and creates/updates as needed)
				// In dry run mode, performs comparison but skips actual API calls
				result, err := manifest.SaveToRemote(providerURL, flags, authToken, dryRun)
				if err != nil {
					return fmt.Errorf("error pushing flags to remote destination: %w", err)
				}

				// Display the results
				displayPushResults(result, providerURL, dryRun)
			default:
				return fmt.Errorf("unsupported URL scheme: %s. Supported schemes are http:// and https://", parsedURL.Scheme)
			}

			return nil
		},
	}

	// Add push-specific flags
	config.AddPushFlags(pushCmd)

	// Add common flags (like --manifest)
	config.AddRootFlags(pushCmd)

	return pushCmd
}

// displayPushResults renders the push operation results with color-coded output
// If dryRun is true, displays what would be pushed instead of what was pushed
func displayPushResults(result *sync.PushResult, destination string, dryRun bool) {
	totalChanges := len(result.Created) + len(result.Updated)

	// Extract just the base URL (domain) for cleaner display
	displayURL := destination
	if parsedURL, err := url.Parse(destination); err == nil {
		// Build base URL with just scheme and host
		displayURL = fmt.Sprintf("%s://%s", parsedURL.Scheme, parsedURL.Host)
	}

	// Determine message based on dry run mode
	if totalChanges == 0 {
		if dryRun {
			pterm.Info.Println("DRY RUN: No changes needed - all flags are already up to date.")
		} else {
			pterm.Success.Println("No changes needed - all flags are already up to date.")
		}
		return
	}

	if dryRun {
		pterm.Info.Printf("DRY RUN: Would push %d flag(s) to %s\n\n", totalChanges, displayURL)
	} else {
		pterm.Success.Printf("Successfully pushed %d flag(s) to %s\n\n", totalChanges, displayURL)
	}

	// Display created flags
	if len(result.Created) > 0 {
		if dryRun {
			pterm.FgCyan.Printf("◆ Would Create (%d):\n", len(result.Created))
		} else {
			pterm.FgGreen.Printf("◆ Created (%d):\n", len(result.Created))
		}

		for _, flag := range result.Created {
			if dryRun {
				pterm.FgCyan.Printf("  + %s", flag.Key)
			} else {
				pterm.FgGreen.Printf("  + %s", flag.Key)
			}

			if flag.Description != "" {
				fmt.Printf(" - %s", flag.Description)
			}
			fmt.Println()

			// Show flag details
			flagJSON, _ := json.MarshalIndent(map[string]any{
				"type":         flag.Type.String(),
				"defaultValue": flag.DefaultValue,
			}, "    ", "  ")
			fmt.Printf("    %s\n", flagJSON)
		}
		fmt.Println()
	}

	// Display updated flags
	if len(result.Updated) > 0 {
		if dryRun {
			pterm.FgMagenta.Printf("◆ Would Update (%d):\n", len(result.Updated))
		} else {
			pterm.FgYellow.Printf("◆ Updated (%d):\n", len(result.Updated))
		}

		for _, flag := range result.Updated {
			if dryRun {
				pterm.FgMagenta.Printf("  ~ %s", flag.Key)
			} else {
				pterm.FgYellow.Printf("  ~ %s", flag.Key)
			}

			if flag.Description != "" {
				fmt.Printf(" - %s", flag.Description)
			}
			fmt.Println()

			// Show flag details
			flagJSON, _ := json.MarshalIndent(map[string]any{
				"type":         flag.Type.String(),
				"defaultValue": flag.DefaultValue,
			}, "    ", "  ")
			fmt.Printf("    %s\n", flagJSON)
		}
		fmt.Println()
	}
}

// pushWithPlugin uses the plugin system to push flags to a remote source
func pushWithPlugin(cmd *cobra.Command, pluginName string, flags *flagset.Flagset, dryRun bool) error {
	logger.Default.Debug(fmt.Sprintf("Using plugin %q for push operation", pluginName))

	// Get the plugin
	p, err := plugin.Get(pluginName)
	if err != nil {
		return fmt.Errorf("failed to get plugin: %w", err)
	}

	// Build plugin configuration
	pluginConfig := plugin.Config{
		BaseURL:   config.GetFlagSourceURL(cmd),
		AuthToken: config.GetAuthToken(cmd),
		Custom:    config.GetPluginConfig(),
	}

	// Configure the plugin
	if err := p.Configure(pluginConfig); err != nil {
		return fmt.Errorf("failed to configure plugin: %w", err)
	}

	// Validate configuration
	if err := p.ValidateConfig(); err != nil {
		return fmt.Errorf("invalid plugin configuration: %w", err)
	}

	// Check if plugin supports push
	if !plugin.HasCapability(p, plugin.CapabilityPush) {
		return fmt.Errorf("plugin %q does not support push operations", pluginName)
	}

	// Push flags using the plugin
	ctx := context.Background()
	result, err := p.Push(flags, plugin.PushOptions{
		Context: ctx,
		DryRun:  dryRun,
	})
	if err != nil {
		return fmt.Errorf("error pushing flags via plugin: %w", err)
	}

	// Display the results
	displayPluginPushResults(result, p.Metadata().Name, dryRun)

	// Report any non-fatal errors
	if len(result.Errors) > 0 {
		pterm.Warning.Println("Some operations had errors:")
		for _, e := range result.Errors {
			pterm.Warning.Printfln("  - %v", e)
		}
	}

	return nil
}

// displayPluginPushResults renders the plugin push operation results
func displayPluginPushResults(result *plugin.PushResult, pluginName string, dryRun bool) {
	totalChanges := len(result.Created) + len(result.Updated)

	// Determine message based on dry run mode
	if totalChanges == 0 {
		if dryRun {
			pterm.Info.Println("DRY RUN: No changes needed - all flags are already up to date.")
		} else {
			pterm.Success.Println("No changes needed - all flags are already up to date.")
		}
		return
	}

	if dryRun {
		pterm.Info.Printf("DRY RUN: Would push %d flag(s) via %s plugin\n\n", totalChanges, pluginName)
	} else {
		pterm.Success.Printf("Successfully pushed %d flag(s) via %s plugin\n\n", totalChanges, pluginName)
	}

	// Display created flags
	if len(result.Created) > 0 {
		if dryRun {
			pterm.FgCyan.Printf("◆ Would Create (%d):\n", len(result.Created))
		} else {
			pterm.FgGreen.Printf("◆ Created (%d):\n", len(result.Created))
		}

		for _, flag := range result.Created {
			if dryRun {
				pterm.FgCyan.Printf("  + %s", flag.Key)
			} else {
				pterm.FgGreen.Printf("  + %s", flag.Key)
			}

			if flag.Description != "" {
				fmt.Printf(" - %s", flag.Description)
			}
			fmt.Println()

			// Show flag details
			flagJSON, _ := json.MarshalIndent(map[string]any{
				"type":         flag.Type.String(),
				"defaultValue": flag.DefaultValue,
			}, "    ", "  ")
			fmt.Printf("    %s\n", flagJSON)
		}
		fmt.Println()
	}

	// Display updated flags
	if len(result.Updated) > 0 {
		if dryRun {
			pterm.FgMagenta.Printf("◆ Would Update (%d):\n", len(result.Updated))
		} else {
			pterm.FgYellow.Printf("◆ Updated (%d):\n", len(result.Updated))
		}

		for _, flag := range result.Updated {
			if dryRun {
				pterm.FgMagenta.Printf("  ~ %s", flag.Key)
			} else {
				pterm.FgYellow.Printf("  ~ %s", flag.Key)
			}

			if flag.Description != "" {
				fmt.Printf(" - %s", flag.Description)
			}
			fmt.Println()

			// Show flag details
			flagJSON, _ := json.MarshalIndent(map[string]any{
				"type":         flag.Type.String(),
				"defaultValue": flag.DefaultValue,
			}, "    ", "  ")
			fmt.Printf("    %s\n", flagJSON)
		}
		fmt.Println()
	}
}
