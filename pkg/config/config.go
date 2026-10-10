package config

import (
	"encoding/json"
	"os"
	"sync"
)

// Config holds user preferences that persist across runs.
type Config struct {
	Language string `json:"language"`
}

// Manager loads and saves the application config from a JSON file.
type Manager struct {
	filePath string
	mu       sync.RWMutex
	cfg      Config
}

// NewManager creates a config manager for the given file path.
func NewManager(filePath string) *Manager {
	if filePath == "" {
		filePath = "config.json"
	}
	m := &Manager{filePath: filePath}
	_ = m.Load()
	return m
}

// Load reads the config from disk. A missing file is not an error.
func (m *Manager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(m.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return json.Unmarshal(data, &m.cfg)
}

// Save writes the config to disk.
func (m *Manager) Save() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	data, err := json.MarshalIndent(m.cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.filePath, data, 0644)
}

// Language returns the saved language code, or "" if none is set.
func (m *Manager) Language() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg.Language
}

// SetLanguage stores the language code and saves the config.
func (m *Manager) SetLanguage(lang string) error {
	m.mu.Lock()
	m.cfg.Language = lang
	m.mu.Unlock()
	return m.Save()
}
