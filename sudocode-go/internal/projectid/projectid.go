package projectid

import (
	"crypto/sha256"
	"fmt"
	"math/big"
	"math"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	nonAlphanumDash = regexp.MustCompile(`[^a-z0-9-]`)
	multiDash       = regexp.MustCompile(`-+`)
	legacyIDRe      = regexp.MustCompile(`^(SPEC|ISSUE)-\d+$`)
	hashIDRe        = regexp.MustCompile(`^[is]-[0-9a-z]{4,8}$`)
)

// Generate produces a deterministic project ID from an absolute path.
// The result matches the TypeScript generateProjectId implementation:
//
//	safeName(basename(path)) + "-" + sha256(path)[:8]
func Generate(absolutePath string) string {
	base := filepath.Base(absolutePath)

	safe := strings.ToLower(base)
	safe = nonAlphanumDash.ReplaceAllString(safe, "-")
	safe = multiDash.ReplaceAllString(safe, "-")
	safe = strings.Trim(safe, "-")
	if len(safe) > 32 {
		safe = safe[:32]
	}

	h := sha256.Sum256([]byte(absolutePath))
	hash := fmt.Sprintf("%x", h[:])[:8]

	return safe + "-" + hash
}

// HashUUIDToBase36 converts a UUID to a base36 hash of the given length,
// matching the TypeScript hashUUIDToBase36 implementation.
func HashUUIDToBase36(uuid string, length int) string {
	clean := strings.ReplaceAll(uuid, "-", "")

	h := sha256.Sum256([]byte(clean))
	hexStr := fmt.Sprintf("%x", h[:])

	hexNeeded := int(math.Ceil(float64(length) * 1.29))
	hexSub := hexStr[:hexNeeded]

	n := new(big.Int)
	n.SetString(hexSub, 16)
	result := strings.ToLower(n.Text(36))

	if len(result) < length {
		result = strings.Repeat("0", length-len(result)) + result
	}
	if len(result) > length {
		result = result[:length]
	}
	return result
}

// GetAdaptiveHashLength returns the hash length based on entity count,
// keeping collision probability under 25%.
func GetAdaptiveHashLength(count int) int {
	switch {
	case count < 980:
		return 4
	case count < 5900:
		return 5
	case count < 35000:
		return 6
	case count < 212000:
		return 7
	default:
		return 8
	}
}

// IsLegacyID checks if an ID uses the legacy SPEC-NNN / ISSUE-NNN format.
func IsLegacyID(id string) bool {
	return legacyIDRe.MatchString(id)
}

// IsHashID checks if an ID uses the hash format (s-xxxx or i-xxxx).
func IsHashID(id string) bool {
	return hashIDRe.MatchString(id)
}

// GetEntityTypeFromID infers "spec" or "issue" from an entity ID.
// Supports both hash-based (s-xxxx, i-xxxx) and legacy (SPEC-xxx, ISSUE-xxx) formats.
func GetEntityTypeFromID(id string) (string, error) {
	switch {
	case strings.HasPrefix(id, "i-"):
		return "issue", nil
	case strings.HasPrefix(id, "s-"):
		return "spec", nil
	case strings.HasPrefix(id, "ISSUE-"):
		return "issue", nil
	case strings.HasPrefix(id, "SPEC-"):
		return "spec", nil
	default:
		return "", fmt.Errorf("cannot infer entity type from ID: %s", id)
	}
}
