package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/open-feature/cli/internal/config"
	"github.com/open-feature/cli/internal/flagset"
	"github.com/open-feature/cli/internal/manifest"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// FlagUsage represents a single usage of a flag in the codebase
type FlagUsage struct {
	FilePath   string `json:"filePath"`
	LineNumber int    `json:"lineNumber"`
	Line       string `json:"line"`
}

// FlagUsageReport represents the usage report for a single flag
type FlagUsageReport struct {
	FlagKey    string      `json:"flagKey"`
	FlagType   string      `json:"flagType"`
	Expiry     string      `json:"expiry,omitempty"`
	IsExpired  bool        `json:"isExpired,omitempty"`
	UsageCount int         `json:"usageCount"`
	Usages     []FlagUsage `json:"usages"`
}

// UsageReport represents the complete usage analysis report
type UsageReport struct {
	TotalFlags     int               `json:"totalFlags"`
	FlagsWithUsage int               `json:"flagsWithUsage"`
	UnusedFlags    int               `json:"unusedFlags"`
	TotalUsages    int               `json:"totalUsages"`
	Reports        []FlagUsageReport `json:"reports"`
}

// Default file extensions to search by language
var defaultExtensions = map[string][]string{
	"go":     {".go"},
	"python": {".py"},
	"java":   {".java"},
	"csharp": {".cs"},
	"nodejs": {".js", ".ts", ".jsx", ".tsx", ".mjs", ".cjs"},
	"react":  {".js", ".ts", ".jsx", ".tsx"},
	"nestjs": {".ts"},
}

func GetManifestUsageCmd() *cobra.Command {
	var searchPath string
	var extensions []string
	var outputFormat string
	var showUnusedOnly bool

	manifestUsageCmd := &cobra.Command{
		Use:   "usage",
		Short: "Analyze flag usage in a codebase",
		Long: `Search for flag references in a codebase and report usage statistics.

This command scans source files for references to flag keys defined in the manifest,
helping identify unused flags and quantify the effort required to remove deprecated flags.

Examples:
  # Scan current directory for flag usage
  openfeature manifest usage --path .

  # Scan with specific file extensions
  openfeature manifest usage --path ./src --ext .ts --ext .tsx

  # Show only unused flags
  openfeature manifest usage --path . --unused-only

  # Output as JSON for tooling integration
  openfeature manifest usage --path . --output json`,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return initializeConfig(cmd, "manifest.usage")
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			manifestPath := config.GetManifestPath(cmd)

			// Load existing manifest
			fs, err := manifest.LoadFlagSet(manifestPath)
			if err != nil {
				return fmt.Errorf("failed to load manifest: %w", err)
			}

			if len(fs.Flags) == 0 {
				pterm.Info.Println("No flags found in manifest")
				return nil
			}

			// Default to current directory if not specified
			if searchPath == "" {
				searchPath = "."
			}

			// Resolve absolute path
			absPath, err := filepath.Abs(searchPath)
			if err != nil {
				return fmt.Errorf("failed to resolve path: %w", err)
			}

			// Use default extensions if none specified
			if len(extensions) == 0 {
				// Use all extensions by default
				extSet := make(map[string]bool)
				for _, exts := range defaultExtensions {
					for _, ext := range exts {
						extSet[ext] = true
					}
				}
				for ext := range extSet {
					extensions = append(extensions, ext)
				}
			}

			// Analyze usage
			report, err := analyzeUsage(fs, absPath, extensions)
			if err != nil {
				return fmt.Errorf("failed to analyze usage: %w", err)
			}

			// Output results
			switch outputFormat {
			case "json":
				return outputJSONUsage(report, showUnusedOnly)
			default:
				return outputTableUsage(report, showUnusedOnly, manifestPath)
			}
		},
	}

	manifestUsageCmd.Flags().StringVarP(&searchPath, "path", "p", ".", "Path to search for flag usage")
	manifestUsageCmd.Flags().StringArrayVarP(&extensions, "ext", "e", nil, "File extensions to search (e.g., --ext .ts --ext .tsx)")
	manifestUsageCmd.Flags().StringVarP(&outputFormat, "output", "o", "table", "Output format: table, json")
	manifestUsageCmd.Flags().BoolVar(&showUnusedOnly, "unused-only", false, "Show only unused flags")

	addStabilityInfo(manifestUsageCmd)

	return manifestUsageCmd
}

// analyzeUsage scans the codebase for flag references
func analyzeUsage(fs *flagset.Flagset, searchPath string, extensions []string) (*UsageReport, error) {
	report := &UsageReport{
		TotalFlags: len(fs.Flags),
		Reports:    make([]FlagUsageReport, 0, len(fs.Flags)),
	}

	// Build extension set for fast lookup
	extSet := make(map[string]bool)
	for _, ext := range extensions {
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		extSet[ext] = true
	}

	// Analyze each flag
	for _, flag := range fs.Flags {
		flagReport := FlagUsageReport{
			FlagKey:   flag.Key,
			FlagType:  flag.Type.String(),
			Expiry:    flag.Expiry,
			IsExpired: flag.IsExpired(),
			Usages:    make([]FlagUsage, 0),
		}

		// Search for this flag key in files
		err := filepath.Walk(searchPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // Skip files we can't access
			}

			// Skip directories and non-matching extensions
			if info.IsDir() {
				// Skip common directories
				base := filepath.Base(path)
				if base == "node_modules" || base == ".git" || base == "vendor" || base == "__pycache__" || base == ".venv" || base == "dist" || base == "build" {
					return filepath.SkipDir
				}
				return nil
			}

			ext := filepath.Ext(path)
			if !extSet[ext] {
				return nil
			}

			// Search file for flag key
			usages, err := searchFileForFlag(path, flag.Key)
			if err != nil {
				return nil // Skip files we can't read
			}

			flagReport.Usages = append(flagReport.Usages, usages...)
			return nil
		})

		if err != nil {
			return nil, fmt.Errorf("error walking path %s: %w", searchPath, err)
		}

		flagReport.UsageCount = len(flagReport.Usages)
		report.Reports = append(report.Reports, flagReport)
		report.TotalUsages += flagReport.UsageCount

		if flagReport.UsageCount > 0 {
			report.FlagsWithUsage++
		}
	}

	report.UnusedFlags = report.TotalFlags - report.FlagsWithUsage

	// Sort reports: expired first, then by usage count (ascending)
	sort.Slice(report.Reports, func(i, j int) bool {
		// Expired flags first
		if report.Reports[i].IsExpired != report.Reports[j].IsExpired {
			return report.Reports[i].IsExpired
		}
		// Then flags with expiry
		if (report.Reports[i].Expiry != "") != (report.Reports[j].Expiry != "") {
			return report.Reports[i].Expiry != ""
		}
		// Then by usage count (unused first)
		if report.Reports[i].UsageCount != report.Reports[j].UsageCount {
			return report.Reports[i].UsageCount < report.Reports[j].UsageCount
		}
		// Finally alphabetically
		return report.Reports[i].FlagKey < report.Reports[j].FlagKey
	})

	return report, nil
}

// searchFileForFlag searches a file for references to a flag key
func searchFileForFlag(filePath, flagKey string) ([]FlagUsage, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var usages []FlagUsage
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Check if line contains the flag key
		// Look for common patterns: "flagKey", 'flagKey', `flagKey`, .flagKey, FlagKey (Pascal case)
		if containsFlagKey(line, flagKey) {
			usages = append(usages, FlagUsage{
				FilePath:   filePath,
				LineNumber: lineNum,
				Line:       strings.TrimSpace(line),
			})
		}
	}

	return usages, scanner.Err()
}

// containsFlagKey checks if a line contains a flag key reference
func containsFlagKey(line, flagKey string) bool {
	// Direct string match (quoted or as identifier)
	if strings.Contains(line, fmt.Sprintf(`"%s"`, flagKey)) ||
		strings.Contains(line, fmt.Sprintf(`'%s'`, flagKey)) ||
		strings.Contains(line, fmt.Sprintf("`%s`", flagKey)) {
		return true
	}

	// Check for camelCase version (e.g., myFlagKey)
	camelCase := toCamelCase(flagKey)
	if strings.Contains(line, camelCase) {
		return true
	}

	// Check for PascalCase version (e.g., MyFlagKey)
	pascalCase := toPascalCase(flagKey)
	if strings.Contains(line, pascalCase) {
		return true
	}

	// Check for SCREAMING_SNAKE_CASE version (e.g., MY_FLAG_KEY)
	screamingSnake := toScreamingSnakeCase(flagKey)
	if strings.Contains(line, screamingSnake) {
		return true
	}

	// Check for snake_case version (e.g., my_flag_key)
	snakeCase := toSnakeCase(flagKey)
	if strings.Contains(line, snakeCase) {
		return true
	}

	return false
}

// Case conversion helpers
func toCamelCase(s string) string {
	parts := splitKey(s)
	if len(parts) == 0 {
		return s
	}
	result := strings.ToLower(parts[0])
	for _, part := range parts[1:] {
		if len(part) > 0 {
			result += strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
		}
	}
	return result
}

func toPascalCase(s string) string {
	parts := splitKey(s)
	var result string
	for _, part := range parts {
		if len(part) > 0 {
			result += strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
		}
	}
	return result
}

func toScreamingSnakeCase(s string) string {
	parts := splitKey(s)
	for i, part := range parts {
		parts[i] = strings.ToUpper(part)
	}
	return strings.Join(parts, "_")
}

func toSnakeCase(s string) string {
	parts := splitKey(s)
	for i, part := range parts {
		parts[i] = strings.ToLower(part)
	}
	return strings.Join(parts, "_")
}

func splitKey(s string) []string {
	// Split on common delimiters: -, _, or camelCase boundaries
	var parts []string
	var current strings.Builder

	for i, r := range s {
		if r == '-' || r == '_' {
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		} else if i > 0 && r >= 'A' && r <= 'Z' {
			// CamelCase boundary
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
			current.WriteRune(r)
		} else {
			current.WriteRune(r)
		}
	}

	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}

// outputTableUsage outputs the usage report as a table
func outputTableUsage(report *UsageReport, showUnusedOnly bool, manifestPath string) error {
	pterm.DefaultSection.Println(fmt.Sprintf("Flag Usage Analysis for %s", manifestPath))

	// Summary
	pterm.Info.Printf("Total flags: %d | With usage: %d | Unused: %d | Total references: %d\n\n",
		report.TotalFlags, report.FlagsWithUsage, report.UnusedFlags, report.TotalUsages)

	// Table data
	tableData := pterm.TableData{
		{"Flag Key", "Type", "Expiry", "Usages", "Status"},
	}

	for _, r := range report.Reports {
		if showUnusedOnly && r.UsageCount > 0 {
			continue
		}

		// Format status
		var status string
		if r.IsExpired {
			status = pterm.FgRed.Sprint("EXPIRED")
		} else if r.Expiry != "" {
			status = pterm.FgYellow.Sprint("EXPIRING")
		} else if r.UsageCount == 0 {
			status = pterm.FgGray.Sprint("UNUSED")
		} else {
			status = pterm.FgGreen.Sprint("IN USE")
		}

		// Format expiry
		expiry := "-"
		if r.Expiry != "" {
			if r.IsExpired {
				expiry = pterm.FgRed.Sprintf("%s", r.Expiry)
			} else {
				expiry = pterm.FgYellow.Sprintf("%s", r.Expiry)
			}
		}

		// Format flag key
		key := r.FlagKey
		if r.IsExpired {
			key = pterm.FgRed.Sprintf("%s", r.FlagKey)
		} else if r.Expiry != "" {
			key = pterm.FgYellow.Sprintf("%s", r.FlagKey)
		}

		tableData = append(tableData, []string{
			key,
			r.FlagType,
			expiry,
			fmt.Sprintf("%d", r.UsageCount),
			status,
		})
	}

	if len(tableData) > 1 {
		_ = pterm.DefaultTable.WithHasHeader().WithData(tableData).Render()
	} else {
		pterm.Info.Println("No flags match the filter criteria")
	}

	// Show detailed usage for flags with few usages (to help with removal)
	fmt.Println()
	for _, r := range report.Reports {
		if r.UsageCount > 0 && r.UsageCount <= 5 && (r.IsExpired || r.Expiry != "") {
			pterm.FgYellow.Printf("📍 %s (%d usage%s):\n", r.FlagKey, r.UsageCount, pluralize(r.UsageCount))
			for _, u := range r.Usages {
				relPath, _ := filepath.Rel(".", u.FilePath)
				if relPath == "" {
					relPath = u.FilePath
				}
				fmt.Printf("   %s:%d\n", relPath, u.LineNumber)
				// Truncate long lines
				line := u.Line
				if len(line) > 80 {
					line = line[:77] + "..."
				}
				pterm.FgGray.Printf("      %s\n", line)
			}
			fmt.Println()
		}
	}

	return nil
}

func pluralize(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// outputJSONUsage outputs the usage report as JSON
func outputJSONUsage(report *UsageReport, showUnusedOnly bool) error {
	output := report
	if showUnusedOnly {
		filteredReports := make([]FlagUsageReport, 0)
		for _, r := range report.Reports {
			if r.UsageCount == 0 {
				filteredReports = append(filteredReports, r)
			}
		}
		output = &UsageReport{
			TotalFlags:     report.TotalFlags,
			FlagsWithUsage: report.FlagsWithUsage,
			UnusedFlags:    report.UnusedFlags,
			TotalUsages:    report.TotalUsages,
			Reports:        filteredReports,
		}
	}

	jsonBytes, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling JSON: %w", err)
	}
	fmt.Println(string(jsonBytes))
	return nil
}
