package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"example.com/shop/services/identity/internal/platform/config"
	"example.com/shop/services/identity/internal/platform/database"
	"example.com/shop/services/identity/migrations"
)

const usage = `usage: migrate <command>

commands:
  up        apply every pending migration
  down [n]  revert the last n migrations (default 1)
  version   print the current schema version`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New(usage)
	}
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	db, err := database.Open(ctx, database.Options{
		DSN:             cfg.DatabaseURL,
		MaxOpenConns:    2,
		MaxIdleConns:    2,
		ConnMaxLifetime: time.Minute,
	})
	if err != nil {
		return err
	}
	defer db.Close()

	switch args[0] {
	case "up":
		applied, err := database.MigrateUp(ctx, db, migrations.FS)
		for _, v := range applied {
			fmt.Printf("applied %d\n", v)
		}
		if err != nil {
			return err
		}
		if len(applied) == 0 {
			fmt.Println("no pending migrations")
		}
	case "down":
		steps := 1
		if len(args) > 1 {
			n, err := strconv.Atoi(args[1])
			if err != nil || n < 1 {
				return fmt.Errorf("down expects a positive number of steps, got %q", args[1])
			}
			steps = n
		}
		reverted, err := database.MigrateDown(ctx, db, migrations.FS, steps)
		for _, v := range reverted {
			fmt.Printf("reverted %d\n", v)
		}
		if err != nil {
			return err
		}
	case "version":
		version, dirty, err := database.MigrationVersion(ctx, db)
		if err != nil {
			return err
		}
		if dirty {
			fmt.Printf("%d (dirty)\n", version)
		} else {
			fmt.Println(version)
		}
	default:
		return errors.New(usage)
	}
	return nil
}
