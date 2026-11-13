package migrations

import (
	"context"
	"embed"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"io/fs"
	"sort"
	"strconv"
	"strings"
)

//go:embed *.sql
var files embed.FS

type migration struct {
	version  int
	up, down string
}

func sequence() ([]migration, error) {
	names, err := fs.Glob(files, "*.up.sql")
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	result := []migration{}
	for _, name := range names {
		v, e := strconv.Atoi(strings.Split(name, "_")[0])
		if e != nil {
			return nil, e
		}
		u, e := files.ReadFile(name)
		if e != nil {
			return nil, e
		}
		d, e := files.ReadFile(strings.TrimSuffix(name, ".up.sql") + ".down.sql")
		if e != nil {
			return nil, e
		}
		result = append(result, migration{v, string(u), string(d)})
	}
	return result, nil
}

// Apply explicitly migrates all known versions; down removes the development schema.
func Apply(ctx context.Context, pool *pgxpool.Pool, direction string) error {
	if direction != "up" && direction != "down" {
		return fmt.Errorf("migration direction must be up or down")
	}
	steps, err := sequence()
	if err != nil {
		return err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext(current_database() || current_schema() || 'novel-migrations'))"); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "CREATE TABLE IF NOT EXISTS schema_migrations(version integer PRIMARY KEY)"); err != nil {
		return err
	}
	if direction == "down" {
		for i, j := 0, len(steps)-1; i < j; i, j = i+1, j-1 {
			steps[i], steps[j] = steps[j], steps[i]
		}
	}
	for _, step := range steps {
		var exists bool
		if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)", step.version).Scan(&exists); err != nil {
			return err
		}
		if direction == "up" && !exists {
			if _, err = tx.Exec(ctx, step.up); err != nil {
				return fmt.Errorf("migration %d: %w", step.version, err)
			}
			if _, err = tx.Exec(ctx, "INSERT INTO schema_migrations VALUES($1)", step.version); err != nil {
				return err
			}
		}
		if direction == "down" && exists {
			if _, err = tx.Exec(ctx, step.down); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, "DELETE FROM schema_migrations WHERE version=$1", step.version); err != nil {
				return err
			}
		}
	}
	return tx.Commit(ctx)
}
func Check(ctx context.Context, pool *pgxpool.Pool) error {
	steps, err := sequence()
	if err != nil {
		return err
	}
	var version int
	if err = pool.QueryRow(ctx, "SELECT COALESCE(max(version),0) FROM schema_migrations").Scan(&version); err != nil {
		return fmt.Errorf("schema unavailable; run migrate up first")
	}
	if len(steps) == 0 || version != steps[len(steps)-1].version {
		return fmt.Errorf("database schema %d requires explicit migration", version)
	}
	return nil
}
