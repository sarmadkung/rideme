// Command migrate applies database schema changes.
//
// Migrations are an explicit, human-invoked operation — never a side effect of
// starting the API (document 24).
//
// Usage:
//
//	go run ./cmd/migrate up
//	go run ./cmd/migrate down          # destructive
//	go run ./cmd/migrate steps -1
//	go run ./cmd/migrate version
//	go run ./cmd/migrate force 1       # clears a dirty state
package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/sarmadkung/rideme/services/api/pkg/config"
	"github.com/sarmadkung/rideme/services/api/pkg/database"
)

const usage = `usage: migrate <up|down|steps N|version|force N>

  up         apply every pending migration
  down       roll back every migration (destructive)
  steps N    move N migrations forward, or backward when negative
  version    print the applied version and whether the schema is dirty
  force N    set the version without running migrations, to clear a dirty state
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return fmt.Errorf("no command given")
	}

	config.LoadDotEnv()

	cfg, err := config.LoadMigrationConfigFromEnv()
	if err != nil {
		return err
	}

	if _, err := os.Stat(cfg.SourcePath); err != nil {
		return fmt.Errorf("migrations directory %q not found — run from services/api "+
			"or set MIGRATIONS_PATH", cfg.SourcePath)
	}

	migrator, err := database.NewMigrator(cfg.SourcePath, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer func() { _ = migrator.Close() }()

	switch args[0] {
	case "up":
		if err := migrator.Up(); err != nil {
			return err
		}
		return printVersion(migrator)

	case "down":
		if err := migrator.Down(); err != nil {
			return err
		}
		fmt.Println("all migrations rolled back")
		return nil

	case "steps":
		if len(args) < 2 {
			return fmt.Errorf("steps requires a count, e.g. `steps -1`")
		}
		n, err := strconv.Atoi(args[1])
		if err != nil {
			return fmt.Errorf("steps count must be an integer: %w", err)
		}
		if err := migrator.Steps(n); err != nil {
			return err
		}
		return printVersion(migrator)

	case "version":
		return printVersion(migrator)

	case "force":
		if len(args) < 2 {
			return fmt.Errorf("force requires a version")
		}
		n, err := strconv.Atoi(args[1])
		if err != nil {
			return fmt.Errorf("version must be an integer: %w", err)
		}
		if err := migrator.Force(n); err != nil {
			return err
		}
		return printVersion(migrator)

	default:
		fmt.Fprint(os.Stderr, usage)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func printVersion(m *database.Migrator) error {
	version, dirty, err := m.Version()
	if err != nil {
		return err
	}
	if dirty {
		// A dirty schema means a migration failed part-way. Automatic recovery
		// would risk applying half a change twice.
		fmt.Printf("version %d (DIRTY — resolve manually, then `migrate force <version>`)\n", version)
		return fmt.Errorf("schema is dirty")
	}
	fmt.Printf("version %d\n", version)
	return nil
}
