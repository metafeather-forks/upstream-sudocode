package registry

import (
	"context"
	"fmt"
	"os"

	"encore.app/sudocode-go/internal/sudocodefile"
	"encore.app/sudocode-go/sync"
)

// RepairParams controls the repair operation.
type RepairParams struct {
	Fix     bool `json:"fix"`     // Actually fix broken links (default: dry-run)
	Rebuild bool `json:"rebuild"` // Rebuild projects.json from scratch
}

// RepairIssue describes a detected problem.
type RepairIssue struct {
	ProjectID   string `json:"projectId"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

// RepairAction describes a fix that was applied.
type RepairAction struct {
	ProjectID   string `json:"projectId"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

// RepairResponse contains the results of a repair operation.
type RepairResponse struct {
	Issues  []RepairIssue  `json:"issues"`
	Actions []RepairAction `json:"actions"`
	Rebuilt bool           `json:"rebuilt"`
}

//encore:api public method=POST path=/registry/repair
func Repair(ctx context.Context, params *RepairParams) (*RepairResponse, error) {
	cfg, err := store.Load()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	resp := &RepairResponse{
		Issues:  []RepairIssue{},
		Actions: []RepairAction{},
	}

	if params.Rebuild {
		return repairRebuild(cfg, resp)
	}

	// Scan all registered projects
	var toRemove []string

	for id, project := range cfg.Projects {
		// 1. Check if sudocodeDir exists
		if _, err := os.Stat(project.SudocodeDir); err != nil {
			if os.IsNotExist(err) {
				resp.Issues = append(resp.Issues, RepairIssue{
					ProjectID:   id,
					Type:        "sudocode_dir_missing",
					Description: fmt.Sprintf("sudocodeDir %s does not exist", project.SudocodeDir),
				})
				if params.Fix {
					toRemove = append(toRemove, id)
					resp.Actions = append(resp.Actions, RepairAction{
						ProjectID:   id,
						Type:        "removed_from_registry",
						Description: fmt.Sprintf("removed %s from registry (sudocodeDir missing)", id),
					})
				}
				continue
			}
			resp.Issues = append(resp.Issues, RepairIssue{
				ProjectID:   id,
				Type:        "sudocode_dir_error",
				Description: fmt.Sprintf("cannot stat sudocodeDir: %v", err),
			})
			continue
		}

		// 2. Read projectdir back-link from config.local.json
		localCfg, err := sync.ReadLocalConfig(project.SudocodeDir)
		if err != nil {
			resp.Issues = append(resp.Issues, RepairIssue{
				ProjectID:   id,
				Type:        "back_link_read_error",
				Description: fmt.Sprintf("cannot read config.local.json: %v", err),
			})
			continue
		}

		projectDir := localCfg.ProjectDir
		if projectDir == "" {
			resp.Issues = append(resp.Issues, RepairIssue{
				ProjectID:   id,
				Type:        "back_link_missing",
				Description: "config.local.json has no projectdir field",
			})
			continue
		}

		// 3. Check if projectdir target exists
		if _, err := os.Stat(projectDir); err != nil {
			if os.IsNotExist(err) {
				resp.Issues = append(resp.Issues, RepairIssue{
					ProjectID:   id,
					Type:        "back_link_target_missing",
					Description: fmt.Sprintf("projectdir target %s does not exist", projectDir),
				})
				continue
			}
		}

		// 4. Check if target has .sudocode file/dir pointing back to this sudocode dir
		resolved, resolveErr := sudocodefile.ResolveSudocodeDir(projectDir)
		if resolveErr != nil {
			// No .sudocode at all in the project dir
			resp.Issues = append(resp.Issues, RepairIssue{
				ProjectID:   id,
				Type:        "back_link_target_no_sudocode",
				Description: fmt.Sprintf("projectdir target %s has no .sudocode: %v", projectDir, resolveErr),
			})
			if params.Fix {
				// Create .sudocode pointer file if external, or just report if co-located
				if err := sudocodefile.WriteSudocodeFile(projectDir, project.SudocodeDir); err == nil {
					resp.Actions = append(resp.Actions, RepairAction{
						ProjectID:   id,
						Type:        "created_sudocode_file",
						Description: fmt.Sprintf("created .sudocode file in %s", projectDir),
					})
				}
			}
			continue
		}

		// 5. Check forward link matches
		if resolved != project.SudocodeDir {
			resp.Issues = append(resp.Issues, RepairIssue{
				ProjectID:   id,
				Type:        "forward_link_mismatch",
				Description: fmt.Sprintf(".sudocode in %s resolves to %s, expected %s", projectDir, resolved, project.SudocodeDir),
			})
			if params.Fix {
				if err := sudocodefile.WriteSudocodeFile(projectDir, project.SudocodeDir); err == nil {
					resp.Actions = append(resp.Actions, RepairAction{
						ProjectID:   id,
						Type:        "fixed_forward_link",
						Description: fmt.Sprintf("updated .sudocode file in %s to point to %s", projectDir, project.SudocodeDir),
					})
				}
			}
		}
	}

	// Apply removals
	if params.Fix && len(toRemove) > 0 {
		_, err := store.Update(func(c *ProjectsConfig) error {
			for _, id := range toRemove {
				delete(c.Projects, id)
				// Clean up recent list
				filtered := make([]string, 0, len(c.RecentProjects))
				for _, rid := range c.RecentProjects {
					if rid != id {
						filtered = append(filtered, rid)
					}
				}
				c.RecentProjects = filtered
				if c.CurrentProjectID == id {
					c.CurrentProjectID = ""
				}
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("save after removals: %w", err)
		}
	}

	return resp, nil
}

// repairRebuild rebuilds projects.json by validating all entries,
// keeping only those whose sudocodeDir exists and has a valid config.
func repairRebuild(cfg ProjectsConfig, resp *RepairResponse) (*RepairResponse, error) {
	validProjects := make(map[string]ProjectInfo)

	for id, project := range cfg.Projects {
		// Check sudocodeDir exists
		if _, err := os.Stat(project.SudocodeDir); err != nil {
			resp.Issues = append(resp.Issues, RepairIssue{
				ProjectID:   id,
				Type:        "sudocode_dir_missing",
				Description: fmt.Sprintf("removed during rebuild: sudocodeDir %s does not exist", project.SudocodeDir),
			})
			continue
		}

		// Verify config.json has a projectId
		projCfg, err := sync.ReadConfig(project.SudocodeDir)
		if err != nil || projCfg.ProjectID == "" {
			resp.Issues = append(resp.Issues, RepairIssue{
				ProjectID:   id,
				Type:        "no_project_id",
				Description: fmt.Sprintf("removed during rebuild: no projectId in config.json"),
			})
			continue
		}

		// Use the projectId from config.json as the canonical ID
		validProjects[projCfg.ProjectID] = ProjectInfo{
			ID:           projCfg.ProjectID,
			Name:         project.Name,
			SudocodeDir:  project.SudocodeDir,
			RegisteredAt: project.RegisteredAt,
			LastOpenedAt: project.LastOpenedAt,
			Favorite:     project.Favorite,
		}
	}

	// Rebuild recent list (keep only valid IDs, preserve order)
	var recentFiltered []string
	for _, rid := range cfg.RecentProjects {
		if _, ok := validProjects[rid]; ok {
			recentFiltered = append(recentFiltered, rid)
		}
	}
	if recentFiltered == nil {
		recentFiltered = []string{}
	}

	// Fix currentProjectID
	currentID := cfg.CurrentProjectID
	if _, ok := validProjects[currentID]; !ok {
		currentID = ""
	}

	newCfg := ProjectsConfig{
		Version:          2,
		Projects:         validProjects,
		RecentProjects:   recentFiltered,
		CurrentProjectID: currentID,
		Settings:         cfg.Settings,
	}

	if err := store.Save(newCfg); err != nil {
		return nil, fmt.Errorf("save rebuilt config: %w", err)
	}

	resp.Rebuilt = true
	return resp, nil
}
