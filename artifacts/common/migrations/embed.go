package migrations

import (
	"embed"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

//go:embed *.sql
var files embed.FS

type Migration struct {
	Version int64
	SQL     string
}

// List returns embedded migrations in ascending, unique version order.
func List() ([]Migration, error) {
	entries, err := files.ReadDir(".")
	if err != nil {
		return nil, err
	}
	versions := make([]Migration, 0, len(entries))
	seen := make(map[int64]bool)
	for _, entry := range entries {
		prefix, _, found := strings.Cut(entry.Name(), "_")
		version, err := strconv.ParseInt(prefix, 10, 64)
		if !found || err != nil || version <= 0 || seen[version] {
			return nil, fmt.Errorf("invalid or duplicate migration version: %s", entry.Name())
		}
		body, err := files.ReadFile(entry.Name())
		if err != nil {
			return nil, err
		}
		seen[version] = true
		versions = append(versions, Migration{Version: version, SQL: string(body)})
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i].Version < versions[j].Version })
	return versions, nil
}
