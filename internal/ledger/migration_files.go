package ledger

import (
	"crypto/sha256"
	"embed"
	"fmt"
	"io/fs"
	"slices"
	"strings"
)

//go:embed all:sql
var migrationFS embed.FS

type embeddedMigration struct {
	version string
	body    []byte
	digest  string
}

// loadMigrations reads every embedded migration in filename order, which is
// the order they apply in.
func loadMigrations() ([]embeddedMigration, error) {
	files, err := fs.Glob(migrationFS, "sql/*.sql")
	if err != nil {
		return nil, fmt.Errorf("list migrations: %w", err)
	}
	slices.Sort(files)
	return embeddedMigrations(migrationFS, files)
}

func embeddedMigrations(filesystem fs.FS, files []string) ([]embeddedMigration, error) {
	out := make([]embeddedMigration, 0, len(files))
	for _, file := range files {
		body, err := fs.ReadFile(filesystem, file)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", file, err)
		}
		version := strings.TrimSuffix(strings.TrimPrefix(file, "sql/"), ".sql")
		out = append(out, embeddedMigration{
			version: version,
			body:    body,
			digest:  digestBytes(body),
		})
	}
	return out, nil
}

func digestBytes(body []byte) string {
	digest := sha256.Sum256(body)
	return fmt.Sprintf("%x", digest[:])
}
