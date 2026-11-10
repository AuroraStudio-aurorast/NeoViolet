package config

// LibraryConfig stores folders to scan; the session queue is independent.
type LibraryConfig struct {
	Roots []string `json:"roots"`
}
