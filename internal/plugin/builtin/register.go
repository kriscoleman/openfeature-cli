// Package builtin imports all built-in plugins to ensure they are registered
package builtin

// Import all built-in plugins to trigger their init() functions
import (
	// Default plugin - uses OpenFeature Manifest Management API
	_ "github.com/open-feature/cli/internal/plugin/builtin/devcycle"
)

// Init is called to ensure the builtin package is imported
// This is a no-op but ensures the import side effects run
func Init() {
	// Built-in plugins are registered via their init() functions
}
