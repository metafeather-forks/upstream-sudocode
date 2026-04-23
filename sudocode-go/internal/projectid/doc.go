// Package projectid implements deterministic project ID generation.
// IDs are composed of a sanitized directory basename and a truncated
// SHA-256 hash of the absolute path.
package projectid
