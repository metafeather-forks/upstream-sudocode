package registry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// ProjectInfo holds metadata for a registered project.
// Note: path field was removed in version 2. The project path is now
// derived from the projectdir back-link in config.local.json.
type ProjectInfo struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	SudocodeDir  string `json:"sudocodeDir"`
	RegisteredAt string `json:"registeredAt"`
	LastOpenedAt string `json:"lastOpenedAt"`
	Favorite     bool   `json:"favorite"`
}

// Settings holds registry-level settings.
type Settings struct {
	MaxRecentProjects   int  `json:"maxRecentProjects"`
	AutoOpenLastProject bool `json:"autoOpenLastProject"`
}

// ProjectsConfig is the top-level structure of projects.json.
type ProjectsConfig struct {
	Version          int                    `json:"version"`
	Projects         map[string]ProjectInfo `json:"projects"`
	RecentProjects   []string               `json:"recentProjects"`
	CurrentProjectID string                 `json:"currentProjectId,omitempty"`
	Settings         Settings               `json:"settings"`
}

func defaultConfig() ProjectsConfig {
	return ProjectsConfig{
		Version:        2,
		Projects:       make(map[string]ProjectInfo),
		RecentProjects: []string{},
		Settings: Settings{
			MaxRecentProjects:   10,
			AutoOpenLastProject: false,
		},
	}
}

// Store provides thread-safe read/write access to projects.json.
type Store struct {
	mu   sync.Mutex
	path string // full path to projects.json
}

// NewStore creates a Store. basePath is the directory containing projects.json
// (e.g. ~/.config/sudocode).
func NewStore(basePath string) *Store {
	return &Store{path: filepath.Join(basePath, "projects.json")}
}

// Load reads and parses projects.json, returning a default config if the file
// does not exist.
func (s *Store) Load() (ProjectsConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

func (s *Store) loadLocked() (ProjectsConfig, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultConfig(), nil
		}
		return ProjectsConfig{}, err
	}
	var cfg ProjectsConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		// corrupted — return default
		return defaultConfig(), nil
	}
	if cfg.Projects == nil {
		cfg.Projects = make(map[string]ProjectInfo)
	}
	if cfg.RecentProjects == nil {
		cfg.RecentProjects = []string{}
	}

	// Migrate v1 → v2: bump version (path field is already dropped by the
	// struct definition — any "path" keys in JSON are silently ignored during
	// unmarshal). The file will be re-written as v2 on the next Save.
	if cfg.Version < 2 {
		cfg.Version = 2
	}

	return cfg, nil
}

// Save atomically writes the config to projects.json.
func (s *Store) Save(cfg ProjectsConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked(cfg)
}

func (s *Store) saveLocked(cfg ProjectsConfig) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// Update loads the config, applies fn, and saves it back atomically.
func (s *Store) Update(fn func(*ProjectsConfig) error) (ProjectsConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg, err := s.loadLocked()
	if err != nil {
		return ProjectsConfig{}, err
	}
	if err := fn(&cfg); err != nil {
		return ProjectsConfig{}, err
	}
	if err := s.saveLocked(cfg); err != nil {
		return ProjectsConfig{}, err
	}
	return cfg, nil
}
