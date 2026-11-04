package storage

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/fstest"
)

// fakeMigrationDB records statements and keeps schema_migrations in memory.
type fakeMigrationDB struct {
	execs   []string
	applied []appliedMigration
	failOn  string // Exec fails when the statement contains this
}

func (f *fakeMigrationDB) Exec(_ context.Context, q string, args ...any) error {
	if f.failOn != "" && strings.Contains(q, f.failOn) {
		return errors.New("boom")
	}
	f.execs = append(f.execs, q)
	if strings.HasPrefix(q, "INSERT INTO schema_migrations") {
		f.applied = append(f.applied, appliedMigration{Version: int(args[0].(uint32)), Checksum: args[2].(string)})
	}
	return nil
}

func (f *fakeMigrationDB) AppliedMigrations(context.Context) ([]appliedMigration, error) {
	return append([]appliedMigration(nil), f.applied...), nil
}

func realMigrations(t *testing.T) []migration {
	t.Helper()
	all, err := loadMigrations(migrationFS)
	if err != nil {
		t.Fatal(err)
	}
	return all
}

func TestEmbeddedMigrationsLoadInOrder(t *testing.T) {
	all := realMigrations(t)
	if len(all) < 2 || all[0].Name != "create_spans" || all[1].Name != "create_trace_summary" {
		t.Fatalf("unexpected migrations: %+v", all)
	}
	// 0001 creates the table with the initial TTL; the collector reconciles it.
	if !strings.Contains(statement(all[0].SQL), "TTL toDateTime(start_time) + INTERVAL 90 DAY") {
		t.Error("0001 should carry the initial 90-day TTL")
	}
}

// ClickHouse has no DDL transactions or locks: two collectors may run the
// same migration concurrently, so each must be safe to apply twice.
func TestMigrationsAreIdempotent(t *testing.T) {
	for _, m := range realMigrations(t) {
		stmt := strings.ToUpper(statement(m.SQL))
		if strings.Contains(stmt, ";") {
			t.Errorf("%04d_%s: one statement per migration file", m.Version, m.Name)
		}
		if !strings.Contains(stmt, "IF NOT EXISTS") && !strings.Contains(stmt, "IF EXISTS") {
			t.Errorf("%04d_%s: must be idempotent (IF NOT EXISTS / IF EXISTS)", m.Version, m.Name)
		}
	}
}

func TestMigrateFreshThenNoop(t *testing.T) {
	all := realMigrations(t)
	db := &fakeMigrationDB{}
	ran, err := migrate(context.Background(), db, all)
	if err != nil || len(ran) != len(all) {
		t.Fatalf("fresh migrate: ran %v, err %v", ran, err)
	}
	if !strings.Contains(db.execs[0], "CREATE TABLE IF NOT EXISTS schema_migrations") {
		t.Error("schema_migrations must be created first")
	}
	for _, q := range db.execs {
		if strings.Contains(q, "--") || strings.HasSuffix(q, ";") {
			t.Errorf("comments and trailing ; should be stripped: %q", q[:min(len(q), 60)])
		}
	}
	db.execs = nil
	ran, err = migrate(context.Background(), db, all)
	if err != nil || len(ran) != 0 {
		t.Fatalf("second migrate: ran %v, err %v", ran, err)
	}
	if len(db.execs) != 1 { // only the schema_migrations CREATE IF NOT EXISTS
		t.Errorf("second migrate executed %d statements, want 1", len(db.execs))
	}
}

func TestMigrateResumesAfterFailure(t *testing.T) {
	all := realMigrations(t)
	db := &fakeMigrationDB{failOn: "trace_summary"}
	ran, err := migrate(context.Background(), db, all)
	if err == nil || len(ran) != 1 || !strings.Contains(err.Error(), "0002_create_trace_summary") {
		t.Fatalf("partial migrate: ran %v, err %v", ran, err)
	}
	db.failOn = ""
	if ran, err = migrate(context.Background(), db, all); err != nil || len(ran) != 1 || ran[0] != 2 {
		t.Fatalf("resume: ran %v, err %v", ran, err)
	}
}

func TestMigrateRejectsEditedAndUnknownMigrations(t *testing.T) {
	all := realMigrations(t)
	db := &fakeMigrationDB{applied: []appliedMigration{{Version: 1, Checksum: "deadbeef"}}}
	if _, err := migrate(context.Background(), db, all); err == nil || !strings.Contains(err.Error(), "edited after it was applied") {
		t.Errorf("edited migration: err = %v", err)
	}
	db = &fakeMigrationDB{applied: []appliedMigration{{Version: 99, Checksum: "x"}}}
	if _, err := migrate(context.Background(), db, all); err == nil || !strings.Contains(err.Error(), "doesn't know") {
		t.Errorf("unknown migration: err = %v", err)
	}
}

func TestLoadMigrationsValidatesFiles(t *testing.T) {
	sql := &fstest.MapFile{Data: []byte("CREATE TABLE IF NOT EXISTS t (x UInt8) ENGINE = Memory")}
	for name, fsys := range map[string]fstest.MapFS{
		"bad name":  {"migrations/1_x.sql": sql},
		"duplicate": {"migrations/0001_a.sql": sql, "migrations/0001_b.sql": sql},
		"gap":       {"migrations/0001_a.sql": sql, "migrations/0003_c.sql": sql},
	} {
		if _, err := loadMigrations(fsys); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
