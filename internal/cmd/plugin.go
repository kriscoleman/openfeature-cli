package cmd

import (
	"fmt"

	"github.com/open-feature/cli/internal/plugin"
	_ "github.com/open-feature/cli/internal/plugin/builtin" // Register built-in plugins
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// GetPluginCmd returns the plugin command with its subcommands
func GetPluginCmd() *cobra.Command {
	pluginCmd := &cobra.Command{
		Use:   "plugin",
		Short: "Manage sync plugins",
		Long: `Manage sync plugins for the OpenFeature CLI.

Plugins extend the CLI's sync functionality to support different feature flag providers.
Each plugin implements a standard interface for pulling and pushing flag configurations.

Built-in plugins:
  - default: Uses the OpenFeature Manifest Management API specification
  - devcycle: Integrates with DevCycle's feature flag management platform

Use 'openfeature plugin list' to see all available plugins.
Use 'openfeature plugin info <name>' to see details about a specific plugin.`,
	}

	// Add subcommands
	pluginCmd.AddCommand(getPluginListCmd())
	pluginCmd.AddCommand(getPluginInfoCmd())

	return pluginCmd
}

// getPluginListCmd returns the plugin list subcommand
func getPluginListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available sync plugins",
		Long: `List all registered sync plugins.

This command displays information about all available plugins, including their
name, description, and stability level.`,
		Example: `  # List all available plugins
  openfeature plugin list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			plugins := plugin.DefaultManager.GetAll()

			if len(plugins) == 0 {
				pterm.Info.Println("No plugins are currently registered.")
				return nil
			}

			pterm.DefaultSection.Println("Available Sync Plugins")
			return plugin.DefaultManager.PrintPluginsTable()
		},
	}
}

// getPluginInfoCmd returns the plugin info subcommand
func getPluginInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info <plugin-name>",
		Short: "Show detailed information about a plugin",
		Long: `Display detailed information about a specific sync plugin.

This includes the plugin's version, stability, supported capabilities,
and configuration options.`,
		Example: `  # Show info about the default plugin
  openfeature plugin info default

  # Show info about the DevCycle plugin
  openfeature plugin info devcycle`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pluginName := args[0]

			// Get the plugin to display its details
			p, err := plugin.Get(pluginName)
			if err != nil {
				return err
			}

			meta := p.Metadata()

			// Print header
			pterm.DefaultSection.Printf("Plugin: %s", meta.Name)
			fmt.Println()

			// Print basic info
			fmt.Printf("  Version:     %s\n", meta.Version)
			fmt.Printf("  Stability:   %s\n", meta.Stability)
			fmt.Printf("  Description: %s\n", meta.Description)
			fmt.Println()

			// Print capabilities
			fmt.Println("  Capabilities:")
			for _, cap := range meta.Capabilities {
				capDescription := getCapabilityDescription(cap)
				fmt.Printf("    - %s: %s\n", cap, capDescription)
			}

			// Print config schema if available
			if meta.ConfigSchema != nil && len(meta.ConfigSchema.Properties) > 0 {
				fmt.Println()
				fmt.Println("  Configuration Options:")

				for propName, prop := range meta.ConfigSchema.Properties {
					isRequired := false
					for _, r := range meta.ConfigSchema.Required {
						if r == propName {
							isRequired = true
							break
						}
					}

					requiredStr := ""
					if isRequired {
						requiredStr = " (required)"
					}

					fmt.Printf("    %s%s:\n", propName, requiredStr)
					fmt.Printf("      Type: %s\n", prop.Type)
					if prop.Description != "" {
						fmt.Printf("      Description: %s\n", prop.Description)
					}
					if prop.EnvVar != "" {
						fmt.Printf("      Environment Variable: %s\n", prop.EnvVar)
					}
					if prop.Default != nil {
						fmt.Printf("      Default: %v\n", prop.Default)
					}
					if prop.Sensitive {
						fmt.Printf("      Sensitive: yes (value will be masked in logs)\n")
					}
				}
			}

			// Print usage example
			fmt.Println()
			fmt.Println("  Usage:")
			fmt.Printf("    openfeature pull --plugin %s\n", pluginName)
			fmt.Printf("    openfeature push --plugin %s\n", pluginName)

			return nil
		},
	}
}

// getCapabilityDescription returns a human-readable description for a capability
func getCapabilityDescription(cap plugin.Capability) string {
	switch cap {
	case plugin.CapabilityPull:
		return "Can fetch flags from remote source"
	case plugin.CapabilityPush:
		return "Can push flags to remote source"
	case plugin.CapabilityCompare:
		return "Can compare local and remote flags"
	case plugin.CapabilityDelete:
		return "Can delete/archive flags remotely"
	default:
		return "Unknown capability"
	}
}
