package registry

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"encore.app/sudocode-go/internal/sudocodefile"
	syncpkg "encore.app/sudocode-go/sync"
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

	// Migrate v1 → v2: bump version, create back-links (projectId in config.json,
	// projectdir in config.local.json), and write .sudocode files for external projects.
	// The path field is dropped by the struct definition (silently ignored during unmarshal).
	// We re-parse raw JSON to recover the v1 path values for migration.
	if cfg.Version < 2 {
		cfg.Version = 2
		if err := s.migrateV1ToV2(data, cfg); err != nil {
			log.Printf("[registry] v1→v2 migration warning: %v", err)
		}
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

// v1ProjectEntry is used to unmarshal v1 projects.json entries which include
// a "path" field that no longer exists in ProjectInfo.
type v1ProjectEntry struct {
	ID          string `json:"id"`
	Path        string `json:"path"`
	SudocodeDir string `json:"sudocodeDir"`
}

// migrateV1ToV2 creates back-links and .sudocode files for existing v1 projects.
// It is idempotent — safe to re-run if interrupted.
func (s *Store) migrateV1ToV2(rawData []byte, cfg ProjectsConfig) error {
	// Re-parse raw JSON to recover v1 path values
	var raw struct {
		Projects map[string]v1ProjectEntry `json:"projects"`
	}
	if err := json.Unmarshal(rawData, &raw); err != nil {
		return fmt.Errorf("re-parse v1 projects: %w", err)
	}

	for id, entry := range raw.Projects {
		if entry.SudocodeDir == "" {
			continue
		}

		// Check sudocodeDir exists
		if _, err := os.Stat(entry.SudocodeDir); err != nil {
			if os.IsNotExist(err) {
				log.Printf("[registry] v1→v2 migration: skipping %s — sudocodeDir %s does not exist", id, entry.SudocodeDir)
				continue
			}
			log.Printf("[registry] v1→v2 migration: skipping %s — stat error: %v", id, err)
			continue
		}

		// Write projectId to config.json (don't overwrite if already set)
		projCfg, err := syncpkg.ReadConfig(entry.SudocodeDir)
		if err != nil {
			log.Printf("[registry] v1→v2 migration: %s — read config.json error: %v", id, err)
		} else if projCfg.ProjectID == "" {
			projCfg.ProjectID = id
			if err := syncpkg.WriteConfig(entry.SudocodeDir, projCfg); err != nil {
				log.Printf("[registry] v1→v2 migration: %s — write config.json error: %v", id, err)
			}
		}

		// Write projectdir to config.local.json (don't overwrite if already set)
		if entry.Path != "" {
			localCfg, err := syncpkg.ReadLocalConfig(entry.SudocodeDir)
			if err != nil {
				log.Printf("[registry] v1→v2 migration: %s — read config.local.json error: %v", id, err)
			} else if localCfg.ProjectDir == "" {
				localCfg.ProjectDir = entry.Path
				if err := syncpkg.WriteLocalConfig(entry.SudocodeDir, localCfg); err != nil {
					log.Printf("[registry] v1→v2 migration: %s — write config.local.json error: %v", id, err)
				}
			}
		}

		// For external projects, create .sudocode file in the repo
		if entry.Path != "" {
			colocated := filepath.Join(entry.Path, ".sudocode")
			if entry.SudocodeDir != colocated {
				// External — create .sudocode pointer file (only if path dir exists)
				if _, err := os.Stat(entry.Path); err == nil {
					if err := sudocodefile.WriteSudocodeFile(entry.Path, entry.SudocodeDir); err != nil {
						log.Printf("[registry] v1→v2 migration: %s — write .sudocode file error: %v", id, err)
					}
				}
			}
		}
	}

	// Save the migrated v2 config
	return s.saveLocked(cfg)
}
