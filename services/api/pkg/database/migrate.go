package database

import (
	"errors"
	"fmt"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5" // registers the pgx5:// driver
	_ "github.com/golang-migrate/migrate/v4/source/file"     // registers the file:// source
)

// Migrator applies versioned schema changes.
//
// Migrations run only from the explicit `migrate` command — never on API
// startup (document 24). Silent migration on boot means an unreviewed schema
// change ships with any rollout and several replicas race to apply it.
type Migrator struct {
	m *migrate.Migrate
}

// NewMigrator opens a migrator over the given source path and database URL.
func NewMigrator(sourcePath, databaseURL string) (*Migrator, error) {
	m, err := migrate.New("file://"+sourcePath, toPgxURL(databaseURL))
	if err != nil {
		return nil, fmt.Errorf("open migrator: %w", err)
	}
	return &Migrator{m: m}, nil
}

// toPgxURL rewrites the scheme golang-migrate's pgx/v5 driver registers under.
// The same DATABASE_URL therefore serves the pool and the migrator.
func toPgxURL(url string) string {
	for _, scheme := range []string{"postgres://", "postgresql://"} {
		if strings.HasPrefix(url, scheme) {
			return "pgx5://" + strings.TrimPrefix(url, scheme)
		}
	}
	return url
}

// Up applies every pending migration. No pending work is success, not an error.
func (m *Migrator) Up() error {
	if err := m.m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

// Down rolls back every migration. Destructive; intended for local development
// and for proving that migrations are reversible.
func (m *Migrator) Down() error {
	if err := m.m.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("roll back migrations: %w", err)
	}
	return nil
}

// Steps moves n migrations forward (positive) or back (negative).
func (m *Migrator) Steps(n int) error {
	if err := m.m.Steps(n); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("step %d migrations: %w", n, err)
	}
	return nil
}

// Version reports the applied version and whether the last run left the schema
// dirty. A dirty schema must be resolved by a human, never automatically.
func (m *Migrator) Version() (version uint, dirty bool, err error) {
	version, dirty, err = m.m.Version()
	if errors.Is(err, migrate.ErrNilVersion) {
		return 0, false, nil
	}
	return version, dirty, err
}

// Force sets the version without running anything, to recover from a dirty state.
func (m *Migrator) Force(version int) error { return m.m.Force(version) }

func (m *Migrator) Close() error {
	sourceErr, dbErr := m.m.Close()
	return errors.Join(sourceErr, dbErr)
}
