package plugin

import (
	"testing"
)

func TestNewManager(t *testing.T) {
	m := NewManager()
	if m == nil {
		t.Error("NewManager() returned nil")
	}

	if m.plugins == nil {
		t.Error("Manager plugins map is nil")
	}
}

func TestManagerRegister(t *testing.T) {
	m := NewManager()

	err := m.Register(NewMockPlugin)
	if err != nil {
		t.Errorf("Register() returned error: %v", err)
	}

	// Check that the plugin was registered
	if !m.HasPlugin("mock") {
		t.Error("Plugin 'mock' was not registered")
	}
}

func TestManagerRegisterDuplicate(t *testing.T) {
	m := NewManager()

	err := m.Register(NewMockPlugin)
	if err != nil {
		t.Errorf("First Register() returned error: %v", err)
	}

	// Registering the same plugin again should fail
	err = m.Register(NewMockPlugin)
	if err == nil {
		t.Error("Duplicate Register() should return error")
	}
}

func TestManagerRegisterEmptyName(t *testing.T) {
	m := NewManager()

	// Create a factory that returns a plugin with empty name
	factory := func() SyncPlugin {
		mock := NewMockPlugin().(*MockPlugin)
		mock.metadata.Name = ""
		return mock
	}

	err := m.Register(factory)
	if err == nil {
		t.Error("Register() with empty name should return error")
	}
}

func TestManagerGet(t *testing.T) {
	m := NewManager()

	err := m.Register(NewMockPlugin)
	if err != nil {
		t.Fatalf("Register() returned error: %v", err)
	}

	plugin, err := m.Get("mock")
	if err != nil {
		t.Errorf("Get() returned error: %v", err)
	}

	if plugin == nil {
		t.Error("Get() returned nil plugin")
	}

	meta := plugin.Metadata()
	if meta.Name != "mock" {
		t.Errorf("Expected plugin name 'mock', got %q", meta.Name)
	}
}

func TestManagerGetNotFound(t *testing.T) {
	m := NewManager()

	_, err := m.Get("nonexistent")
	if err == nil {
		t.Error("Get() for nonexistent plugin should return error")
	}
}

func TestManagerGetInfo(t *testing.T) {
	m := NewManager()

	err := m.Register(NewMockPlugin)
	if err != nil {
		t.Fatalf("Register() returned error: %v", err)
	}

	info, err := m.GetInfo("mock")
	if err != nil {
		t.Errorf("GetInfo() returned error: %v", err)
	}

	if info.Name != "mock" {
		t.Errorf("Expected plugin name 'mock', got %q", info.Name)
	}

	if info.Stability != StabilityExperimental {
		t.Errorf("Expected stability 'experimental', got %q", info.Stability)
	}
}

func TestManagerList(t *testing.T) {
	m := NewManager()

	// Initially empty
	list := m.List()
	if len(list) != 0 {
		t.Errorf("Expected empty list, got %d plugins", len(list))
	}

	// Add a plugin
	err := m.Register(NewMockPlugin)
	if err != nil {
		t.Fatalf("Register() returned error: %v", err)
	}

	list = m.List()
	if len(list) != 1 {
		t.Errorf("Expected 1 plugin, got %d", len(list))
	}

	if list[0] != "mock" {
		t.Errorf("Expected plugin name 'mock', got %q", list[0])
	}
}

func TestManagerGetAll(t *testing.T) {
	m := NewManager()

	err := m.Register(NewMockPlugin)
	if err != nil {
		t.Fatalf("Register() returned error: %v", err)
	}

	all := m.GetAll()
	if len(all) != 1 {
		t.Errorf("Expected 1 plugin, got %d", len(all))
	}

	info, exists := all["mock"]
	if !exists {
		t.Error("Plugin 'mock' not found in GetAll() result")
	}

	if info.Name != "mock" {
		t.Errorf("Expected plugin name 'mock', got %q", info.Name)
	}
}

func TestManagerHasPlugin(t *testing.T) {
	m := NewManager()

	// Initially no plugins
	if m.HasPlugin("mock") {
		t.Error("HasPlugin('mock') should return false initially")
	}

	// Add a plugin
	err := m.Register(NewMockPlugin)
	if err != nil {
		t.Fatalf("Register() returned error: %v", err)
	}

	if !m.HasPlugin("mock") {
		t.Error("HasPlugin('mock') should return true after registration")
	}

	if m.HasPlugin("nonexistent") {
		t.Error("HasPlugin('nonexistent') should return false")
	}
}

func TestDefaultManager(t *testing.T) {
	// Test that DefaultManager is initialized
	if DefaultManager == nil {
		t.Error("DefaultManager is nil")
	}
}

func TestConvenienceFunctions(t *testing.T) {
	// Create a new manager to test with
	original := DefaultManager
	DefaultManager = NewManager()
	defer func() { DefaultManager = original }()

	// Test Register convenience function
	err := Register(NewMockPlugin)
	if err != nil {
		t.Errorf("Register() returned error: %v", err)
	}

	// Test Get convenience function
	plugin, err := Get("mock")
	if err != nil {
		t.Errorf("Get() returned error: %v", err)
	}
	if plugin == nil {
		t.Error("Get() returned nil plugin")
	}

	// Test List convenience function
	list := List()
	if len(list) != 1 {
		t.Errorf("Expected 1 plugin in list, got %d", len(list))
	}
}
