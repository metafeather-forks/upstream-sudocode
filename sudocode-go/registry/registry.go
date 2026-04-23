package registry

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"time"
)

// store is the package-level Store instance.
// Overridden in tests via SetStore.
var store *Store

func init() {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	store = NewStore(filepath.Join(home, ".config", "sudocode"))
}

// SetStore replaces the package-level store (for testing).
func SetStore(s *Store) {
	store = s
}

// --- Request / Response types ---

type ListResponse struct {
	Projects []ProjectInfo `json:"projects"`
}

type OpenParams struct {
	Path string `json:"path"`
}

type OpenResponse struct {
	Project ProjectInfo `json:"project"`
}

type CloseParams struct {
	ProjectID string `json:"projectId"`
}

type CloseResponse struct {
	OK bool `json:"ok"`
}

type InitParams struct {
	Path string  `json:"path"`
	Name *string `json:"name,omitempty"`
}

type InitResponse struct {
	Project ProjectInfo `json:"project"`
}

type ValidateParams struct {
	Path string `json:"path"`
}

type ValidateResponse struct {
	Valid        bool   `json:"valid"`
	HasSudocode  bool   `json:"hasSudocode"`
	ProjectID    string `json:"projectId,omitempty"`
	ErrorMessage string `json:"errorMessage,omitempty"`
}

type BrowseParams struct {
	Path string `json:"path"`
}

type DirEntry struct {
	Name  string `json:"name"`
	IsDir bool   `json:"isDir"`
}

type BrowseResponse struct {
	CurrentPath string     `json:"currentPath"`
	ParentPath  string     `json:"parentPath"`
	Entries     []DirEntry `json:"entries"`
}

type CurrentResponse struct {
	ProjectID string       `json:"projectId"`
	Project   *ProjectInfo `json:"project,omitempty"`
}

type SetCurrentParams struct {
	ProjectID string `json:"projectId"`
}

type RecentResponse struct {
	Projects []ProjectInfo `json:"projects"`
}

// --- Endpoints ---

//encore:api public method=POST path=/registry/list
func List(ctx context.Context) (*ListResponse, error) {
	cfg, err := store.Load()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	projects := make([]ProjectInfo, 0, len(cfg.Projects))
	for _, p := range cfg.Projects {
		projects = append(projects, p)
	}
	sort.Slice(projects, func(i, j int) bool {
		return projects[i].LastOpenedAt > projects[j].LastOpenedAt
	})
	return &ListResponse{Projects: projects}, nil
}

//encore:api public method=POST path=/registry/open
func Open(ctx context.Context, params *OpenParams) (*OpenResponse, error) {
	absPath, err := filepath.Abs(params.Path)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}
	info, err := os.Stat(absPath)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("path not found or not a directory: %s", absPath)
	}

	id := GenerateProjectID(absPath)
	now := time.Now().UTC().Format(time.RFC3339)

	cfg, err := store.Update(func(c *ProjectsConfig) error {
		p, exists := c.Projects[id]
		if !exists {
			p = ProjectInfo{
				ID:           id,
				Name:         filepath.Base(absPath),
				Path:         absPath,
				SudocodeDir:  filepath.Join(absPath, ".sudocode"),
				RegisteredAt: now,
			}
		}
		p.LastOpenedAt = now
		c.Projects[id] = p
		c.CurrentProjectID = id

		// Update recent list
		recent := []string{id}
		for _, rid := range c.RecentProjects {
			if rid != id {
				recent = append(recent, rid)
			}
		}
		max := c.Settings.MaxRecentProjects
		if max <= 0 {
			max = 10
		}
		if len(recent) > max {
			recent = recent[:max]
		}
		c.RecentProjects = recent
		return nil
	})
	if err != nil {
		return nil, err
	}
	project := cfg.Projects[id]
	return &OpenResponse{Project: project}, nil
}

//encore:api public method=POST path=/registry/close
func Close(ctx context.Context, params *CloseParams) (*CloseResponse, error) {
	_, err := store.Update(func(c *ProjectsConfig) error {
		if _, exists := c.Projects[params.ProjectID]; !exists {
			return fmt.Errorf("project not found: %s", params.ProjectID)
		}
		if c.CurrentProjectID == params.ProjectID {
			c.CurrentProjectID = ""
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &CloseResponse{OK: true}, nil
}

//encore:api public method=POST path=/registry/init
func Init(ctx context.Context, params *InitParams) (*InitResponse, error) {
	absPath, err := filepath.Abs(params.Path)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}
	info, err := os.Stat(absPath)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("path not found or not a directory: %s", absPath)
	}

	sudocodeDir := filepath.Join(absPath, ".sudocode")
	if err := os.MkdirAll(sudocodeDir, 0o755); err != nil {
		return nil, fmt.Errorf("create .sudocode dir: %w", err)
	}

	name := filepath.Base(absPath)
	if params.Name != nil && *params.Name != "" {
		name = *params.Name
	}

	id := GenerateProjectID(absPath)
	now := time.Now().UTC().Format(time.RFC3339)

	cfg, err := store.Update(func(c *ProjectsConfig) error {
		c.Projects[id] = ProjectInfo{
			ID:           id,
			Name:         name,
			Path:         absPath,
			SudocodeDir:  sudocodeDir,
			RegisteredAt: now,
			LastOpenedAt: now,
		}
		c.CurrentProjectID = id
		recent := []string{id}
		for _, rid := range c.RecentProjects {
			if rid != id {
				recent = append(recent, rid)
			}
		}
		max := c.Settings.MaxRecentProjects
		if max <= 0 {
			max = 10
		}
		if len(recent) > max {
			recent = recent[:max]
		}
		c.RecentProjects = recent
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &InitResponse{Project: cfg.Projects[id]}, nil
}

//encore:api public method=POST path=/registry/validate
func Validate(ctx context.Context, params *ValidateParams) (*ValidateResponse, error) {
	absPath, err := filepath.Abs(params.Path)
	if err != nil {
		return &ValidateResponse{Valid: false, ErrorMessage: "invalid path"}, nil
	}
	info, err := os.Stat(absPath)
	if err != nil || !info.IsDir() {
		return &ValidateResponse{Valid: false, ErrorMessage: "path not found or not a directory"}, nil
	}

	sudocodeDir := filepath.Join(absPath, ".sudocode")
	_, err = os.Stat(sudocodeDir)
	hasSudocode := err == nil

	id := GenerateProjectID(absPath)
	return &ValidateResponse{
		Valid:       true,
		HasSudocode: hasSudocode,
		ProjectID:   id,
	}, nil
}

//encore:api public method=POST path=/registry/browse
func Browse(ctx context.Context, params *BrowseParams) (*BrowseResponse, error) {
	absPath, err := filepath.Abs(params.Path)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}
	entries, err := os.ReadDir(absPath)
	if err != nil {
		return nil, fmt.Errorf("read directory: %w", err)
	}
	var dirEntries []DirEntry
	for _, e := range entries {
		dirEntries = append(dirEntries, DirEntry{
			Name:  e.Name(),
			IsDir: e.IsDir(),
		})
	}
	return &BrowseResponse{
		CurrentPath: absPath,
		ParentPath:  filepath.Dir(absPath),
		Entries:     dirEntries,
	}, nil
}

//encore:api public method=POST path=/registry/current
func Current(ctx context.Context) (*CurrentResponse, error) {
	cfg, err := store.Load()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	resp := &CurrentResponse{ProjectID: cfg.CurrentProjectID}
	if p, ok := cfg.Projects[cfg.CurrentProjectID]; ok {
		resp.Project = &p
	}
	return resp, nil
}

//encore:api public method=POST path=/registry/set-current
func SetCurrent(ctx context.Context, params *SetCurrentParams) (*CurrentResponse, error) {
	cfg, err := store.Update(func(c *ProjectsConfig) error {
		if _, exists := c.Projects[params.ProjectID]; !exists {
			return fmt.Errorf("project not found: %s", params.ProjectID)
		}
		c.CurrentProjectID = params.ProjectID
		return nil
	})
	if err != nil {
		return nil, err
	}
	resp := &CurrentResponse{ProjectID: cfg.CurrentProjectID}
	if p, ok := cfg.Projects[cfg.CurrentProjectID]; ok {
		resp.Project = &p
	}
	return resp, nil
}

//encore:api public method=POST path=/registry/recent
func Recent(ctx context.Context) (*RecentResponse, error) {
	cfg, err := store.Load()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	var projects []ProjectInfo
	for _, id := range cfg.RecentProjects {
		if p, ok := cfg.Projects[id]; ok {
			projects = append(projects, p)
		}
	}
	return &RecentResponse{Projects: projects}, nil
}

// openBrowser opens a URL in the default browser.
func openBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "linux":
		return exec.Command("xdg-open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return fmt.Errorf("unsupported platform")
	}
}
