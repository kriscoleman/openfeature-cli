package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/open-feature/cli/internal/config"
	"github.com/open-feature/cli/internal/flagset"
	"github.com/open-feature/cli/internal/manifest"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

func GetManifestListCmd() *cobra.Command {
	manifestListCmd := &cobra.Command{
		Use:   "list",
		Short: "List all flags in the manifest",
		Long:  `Display all flags defined in the manifest file with their configuration.`,
		Args:  cobra.NoArgs,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return initializeConfig(cmd, "manifest.list")
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			manifestPath := config.GetManifestPath(cmd)

			// Load existing manifest
			fs, err := manifest.LoadFlagSet(manifestPath)
			if err != nil {
				return fmt.Errorf("failed to load manifest: %w", err)
			}

			displayFlagList(fs, manifestPath)
			return nil
		},
	}

	// Add command-specific flags
	config.AddManifestListFlags(manifestListCmd)
	addStabilityInfo(manifestListCmd)

	return manifestListCmd
}

// displayFlagList prints a formatted table of all flags in the flagset
func displayFlagList(fs *flagset.Flagset, manifestPath string) {
	if len(fs.Flags) == 0 {
		pterm.Info.Println("No flags found in manifest")
		return
	}

	// Count expired and expiring flags
	var expiredCount, expiringCount int
	for _, flag := range fs.Flags {
		if flag.HasExpiry() {
			if flag.IsExpired() {
				expiredCount++
			} else {
				expiringCount++
			}
		}
	}

	// Print header
	pterm.DefaultSection.Println(fmt.Sprintf("Flags in %s (%d)", manifestPath, len(fs.Flags)))

	// Print deprecation summary if any
	if expiredCount > 0 || expiringCount > 0 {
		if expiredCount > 0 {
			pterm.FgRed.Printf("  ⚠ %d expired flag(s)\n", expiredCount)
		}
		if expiringCount > 0 {
			pterm.FgYellow.Printf("  ⏰ %d flag(s) with scheduled expiry\n", expiringCount)
		}
		fmt.Println()
	}

	// Create table data
	tableData := pterm.TableData{
		{"Key", "Type", "Default Value", "Expiry", "Description"},
	}

	for _, flag := range fs.Flags {
		// Format default value for display
		defaultValueStr := formatValue(flag.DefaultValue)

		// Truncate description if too long
		description := flag.Description
		const maxDescriptionLength = 40

		if len(description) > maxDescriptionLength {
			description = description[:maxDescriptionLength-3] + "..."
		}

		// Format expiry with status indicator
		expiry := formatExpiry(&flag)

		// Format key with deprecation indicator
		key := flag.Key
		if flag.IsExpired() {
			key = pterm.FgRed.Sprintf("%s", flag.Key)
		} else if flag.HasExpiry() {
			key = pterm.FgYellow.Sprintf("%s", flag.Key)
		}

		tableData = append(tableData, []string{
			key,
			flag.Type.String(),
			defaultValueStr,
			expiry,
			description,
		})
	}

	// Render table
	_ = pterm.DefaultTable.WithHasHeader().WithData(tableData).Render()
}

// formatExpiry formats the expiry date with status indicator
func formatExpiry(flag *flagset.Flag) string {
	if flag.Expiry == "" {
		return "-"
	}
	if flag.IsExpired() {
		return pterm.FgRed.Sprintf("%s (expired)", flag.Expiry)
	}
	return pterm.FgYellow.Sprintf("%s", flag.Expiry)
}

// formatValue converts a value to a string representation suitable for display
func formatValue(value any) string {
	switch v := value.(type) {
	case string:
		if len(v) > 30 {
			return fmt.Sprintf("\"%s...\"", v[:27])
		}
		return fmt.Sprintf("\"%s\"", v)
	case bool, int, float64:
		return fmt.Sprintf("%v", v)
	case map[string]any, []any:
		jsonBytes, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		jsonStr := string(jsonBytes)
		if len(jsonStr) > 30 {
			return jsonStr[:27] + "..."
		}
		return jsonStr
	default:
		return fmt.Sprintf("%v", v)
	}
}
