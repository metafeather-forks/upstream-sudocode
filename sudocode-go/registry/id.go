package registry

import "encore.app/sudocode-go/internal/projectid"

// GenerateProjectID produces a deterministic project ID from an absolute path.
func GenerateProjectID(absolutePath string) string {
	return projectid.Generate(absolutePath)
}
