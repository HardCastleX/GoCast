package presets

import (
	"encoding/json"
	"os"
	"sync"
)

// Preset represents the saved configuration for a station.
type Preset struct {
	Host    string `json:"host"`
	Port    string `json:"port"`
	SID     string `json:"sid"`
	User    string `json:"user"`
	Pass    string `json:"pass"`
	Bitrate string `json:"bitrate"`
}

// Manager handles loading and saving presets to JSON.
type Manager struct {
	filePath string
	mu       sync.RWMutex
	presets  map[string]Preset
}

// NewManager creates a presets manager for the given file path.
func NewManager(filePath string) *Manager {
	if filePath == "" {
		filePath = "presets.json"
	}
	m := &Manager{
		filePath: filePath,
		presets:  make(map[string]Preset),
	}
	_ = m.Load()
	return m
}

// Load reads presets from disk.
func (m *Manager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(m.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			m.presets = make(map[string]Preset)
			return nil
		}
		return err
	}

	var loaded map[string]Preset
	if err := json.Unmarshal(data, &loaded); err != nil {
		return err
	}
	m.presets = loaded
	return nil
}

// Save writes current presets to disk.
func (m *Manager) Save() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	data, err := json.MarshalIndent(m.presets, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.filePath, data, 0644)
}

// GetAll returns a copy of all presets.
func (m *Manager) GetAll() map[string]Preset {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]Preset, len(m.presets))
	for k, v := range m.presets {
		result[k] = v
	}
	return result
}

// Get returns a single preset by name.
func (m *Manager) Get(name string) (Preset, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.presets[name]
	return p, ok
}

// Set adds or updates a preset and saves to disk.
func (m *Manager) Set(name string, p Preset) error {
	m.mu.Lock()
	m.presets[name] = p
	m.mu.Unlock()
	return m.Save()
}

// Delete removes a preset and saves to disk.
func (m *Manager) Delete(name string) error {
	m.mu.Lock()
	delete(m.presets, name)
	m.mu.Unlock()
	return m.Save()
}
