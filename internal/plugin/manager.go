package plugin

import (
	"fmt"
	"sort"
	"sync"

	"github.com/pterm/pterm"
)

// PluginFactory is a function that creates a new plugin instance
type PluginFactory func() SyncPlugin

// PluginInfo contains metadata about a registered plugin
type PluginInfo struct {
	Name        string
	Description string
	Stability   Stability
	Factory     PluginFactory
}

// Manager maintains a registry of available sync plugins
type Manager struct {
	mu      sync.RWMutex
	plugins map[string]PluginInfo
}

// NewManager creates a new plugin manager
func NewManager() *Manager {
	return &Manager{
		plugins: make(map[string]PluginInfo),
	}
}

// Register adds a plugin to the registry
func (m *Manager) Register(factory PluginFactory) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Create an instance to get metadata
	plugin := factory()
	meta := plugin.Metadata()

	if meta.Name == "" {
		return fmt.Errorf("plugin name cannot be empty")
	}

	if _, exists := m.plugins[meta.Name]; exists {
		return fmt.Errorf("plugin %q is already registered", meta.Name)
	}

	m.plugins[meta.Name] = PluginInfo{
		Name:        meta.Name,
		Description: meta.Description,
		Stability:   meta.Stability,
		Factory:     factory,
	}

	return nil
}

// Get returns a new instance of the plugin with the given name
func (m *Manager) Get(name string) (SyncPlugin, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	info, exists := m.plugins[name]
	if !exists {
		available := m.listPluginNames()
		return nil, fmt.Errorf("plugin %q not found. Available plugins: %v", name, available)
	}

	return info.Factory(), nil
}

// GetInfo returns metadata about a registered plugin without creating an instance
func (m *Manager) GetInfo(name string) (PluginInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	info, exists := m.plugins[name]
	if !exists {
		return PluginInfo{}, fmt.Errorf("plugin %q not found", name)
	}

	return info, nil
}

// List returns all registered plugin names
func (m *Manager) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.listPluginNames()
}

func (m *Manager) listPluginNames() []string {
	names := make([]string, 0, len(m.plugins))
	for name := range m.plugins {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// GetAll returns all registered plugin info
func (m *Manager) GetAll() map[string]PluginInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy to avoid race conditions
	result := make(map[string]PluginInfo, len(m.plugins))
	for k, v := range m.plugins {
		result[k] = v
	}
	return result
}

// HasPlugin checks if a plugin with the given name is registered
func (m *Manager) HasPlugin(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, exists := m.plugins[name]
	return exists
}

// PrintPluginsTable prints a table of all available plugins with their details
func (m *Manager) PrintPluginsTable() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tableData := [][]string{
		{"Plugin", "Description", "Stability"},
	}

	names := m.listPluginNames()
	for _, name := range names {
		info := m.plugins[name]
		tableData = append(tableData, []string{
			name,
			info.Description,
			string(info.Stability),
		})
	}

	return pterm.DefaultTable.WithHasHeader().WithData(tableData).Render()
}

// PrintPluginDetails prints detailed information about a specific plugin
func (m *Manager) PrintPluginDetails(name string) error {
	plugin, err := m.Get(name)
	if err != nil {
		return err
	}

	meta := plugin.Metadata()

	// Print basic info
	pterm.DefaultSection.Printf("Plugin: %s", meta.Name)

	fmt.Printf("Version:     %s\n", meta.Version)
	fmt.Printf("Stability:   %s\n", meta.Stability)
	fmt.Printf("Description: %s\n", meta.Description)

	// Print capabilities
	fmt.Println("\nCapabilities:")
	for _, cap := range meta.Capabilities {
		fmt.Printf("  - %s\n", cap)
	}

	// Print config schema if available
	if meta.ConfigSchema != nil && len(meta.ConfigSchema.Properties) > 0 {
		fmt.Println("\nConfiguration Options:")
		for propName, prop := range meta.ConfigSchema.Properties {
			required := ""
			for _, r := range meta.ConfigSchema.Required {
				if r == propName {
					required = " (required)"
					break
				}
			}
			fmt.Printf("  %s%s:\n", propName, required)
			fmt.Printf("    Type: %s\n", prop.Type)
			if prop.Description != "" {
				fmt.Printf("    Description: %s\n", prop.Description)
			}
			if prop.EnvVar != "" {
				fmt.Printf("    Environment Variable: %s\n", prop.EnvVar)
			}
			if prop.Default != nil {
				fmt.Printf("    Default: %v\n", prop.Default)
			}
		}
	}

	return nil
}

// DefaultManager is the default instance of the plugin manager
var DefaultManager = NewManager()

// Register is a convenience function to register a plugin with the default manager
func Register(factory PluginFactory) error {
	return DefaultManager.Register(factory)
}

// Get is a convenience function to get a plugin from the default manager
func Get(name string) (SyncPlugin, error) {
	return DefaultManager.Get(name)
}

// List is a convenience function to list plugins from the default manager
func List() []string {
	return DefaultManager.List()
}
