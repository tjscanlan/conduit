package storage

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Schema migrations.
//
// Each file in migrations/ is NNNN_description.sql and holds exactly one
// statement. Applied versions are recorded in schema_migrations with a
// SHA-256 checksum, so editing an applied migration is detected rather than
// silently diverging. ClickHouse has no DDL transactions or locks, so every
// migration must be idempotent (IF NOT EXISTS / IF EXISTS): two collectors
// starting at once may both apply it. TestMigrationsAreIdempotent enforces it.
//
// Never edit or renumber a migration once it has shipped; add a new one.

//go:embed migrations/*.sql
var migrationFS embed.FS

type migration struct {
	Version  int
	Name     string
	SQL      string
	Checksum string
}

type appliedMigration struct {
	Version  int
	Checksum string
}

// migrationDB is the slice of ClickHouse the migrator needs (tests fake it).
type migrationDB interface {
	Exec(ctx context.Context, query string, args ...any) error
	AppliedMigrations(ctx context.Context) ([]appliedMigration, error)
}

const createMigrationsTable = `CREATE TABLE IF NOT EXISTS schema_migrations (
    version    UInt32,
    name       String,
    checksum   String,
    applied_at DateTime DEFAULT now()
) ENGINE = ReplacingMergeTree(applied_at)
ORDER BY version`

var migrationName = regexp.MustCompile(`^(\d{4})_([a-z0-9_]+)\.sql$`)

func loadMigrations(fsys fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(fsys, "migrations")
	if err != nil {
		return nil, err
	}
	var out []migration
	seen := map[int]string{}
	for _, e := range entries {
		m := migrationName.FindStringSubmatch(e.Name())
		if m == nil {
			return nil, fmt.Errorf("migration %q: name must be NNNN_description.sql", e.Name())
		}
		v, _ := strconv.Atoi(m[1])
		if prev, dup := seen[v]; dup {
			return nil, fmt.Errorf("migrations %q and %q share version %d", prev, e.Name(), v)
		}
		seen[v] = e.Name()
		b, err := fs.ReadFile(fsys, "migrations/"+e.Name())
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(b)
		out = append(out, migration{Version: v, Name: m[2], SQL: string(b), Checksum: hex.EncodeToString(sum[:])})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	for i, m := range out {
		if m.Version != i+1 {
			return nil, fmt.Errorf("migration versions must be 1..N without gaps; found %04d at position %d", m.Version, i+1)
		}
	}
	return out, nil
}

// statement strips SQL comments and the trailing semicolon from a migration file.
func statement(sql string) string {
	var lines []string
	for _, l := range strings.Split(sql, "\n") {
		if i := strings.Index(l, "--"); i >= 0 {
			l = l[:i]
		}
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	return strings.TrimSuffix(strings.TrimSpace(strings.Join(lines, "\n")), ";")
}

// migrate applies pending migrations in order and returns the versions applied.
func migrate(ctx context.Context, db migrationDB, all []migration) ([]int, error) {
	if err := db.Exec(ctx, createMigrationsTable); err != nil {
		return nil, fmt.Errorf("create schema_migrations: %w", err)
	}
	applied, err := db.AppliedMigrations(ctx)
	if err != nil {
		return nil, fmt.Errorf("read schema_migrations: %w", err)
	}
	done := map[int]string{}
	for _, a := range applied {
		done[a.Version] = a.Checksum
	}
	byVersion := map[int]migration{}
	for _, m := range all {
		byVersion[m.Version] = m
	}
	for v, sum := range done {
		m, ok := byVersion[v]
		if !ok {
			return nil, fmt.Errorf("database has migration %04d, which this binary doesn't know; is it older than the schema?", v)
		}
		if m.Checksum != sum {
			return nil, fmt.Errorf("migration %04d_%s was edited after it was applied (checksum mismatch); add a new migration instead", v, m.Name)
		}
	}

	var ran []int
	for _, m := range all {
		if _, ok := done[m.Version]; ok {
			continue
		}
		if err := db.Exec(ctx, statement(m.SQL)); err != nil {
			return ran, fmt.Errorf("migration %04d_%s: %w", m.Version, m.Name, err)
		}
		if err := db.Exec(ctx, `INSERT INTO schema_migrations (version, name, checksum) VALUES (?, ?, ?)`,
			uint32(m.Version), m.Name, m.Checksum); err != nil {
			return ran, fmt.Errorf("record migration %04d: %w", m.Version, err)
		}
		ran = append(ran, m.Version)
	}
	return ran, nil
}

// clickhouseMigrationDB adapts a ClickHouseStore to migrationDB.
type clickhouseMigrationDB struct{ s *ClickHouseStore }

func (d clickhouseMigrationDB) Exec(ctx context.Context, q string, args ...any) error {
	return d.s.conn.Exec(ctx, q, args...)
}

func (d clickhouseMigrationDB) AppliedMigrations(ctx context.Context) ([]appliedMigration, error) {
	rows, err := d.s.conn.Query(ctx, `SELECT version, checksum FROM schema_migrations FINAL ORDER BY version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []appliedMigration
	for rows.Next() {
		var v uint32
		var a appliedMigration
		if err := rows.Scan(&v, &a.Checksum); err != nil {
			return nil, err
		}
		a.Version = int(v)
		out = append(out, a)
	}
	return out, rows.Err()
}

// Migrate applies pending schema migrations and returns the versions it ran.
func (s *ClickHouseStore) Migrate(ctx context.Context) ([]int, error) {
	all, err := loadMigrations(migrationFS)
	if err != nil {
		return nil, err
	}
	return migrate(ctx, clickhouseMigrationDB{s}, all)
}
