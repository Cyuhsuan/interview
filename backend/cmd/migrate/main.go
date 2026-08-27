// cmd/migrate is the ordered, reviewable migration/seed runner required
// by backend/AGENTS.md — GORM AutoMigrate is forbidden, and neither
// migrating nor seeding may be invoked automatically at API startup.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"backend/internal/platform/idgen"
	seedrepo "backend/internal/repository/seed"
	seedsvc "backend/internal/service/seed"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: migrate <up|down|seed>")
	}
	cmd := os.Args[1]

	if os.Getenv("APP_ENV") != "production" {
		_ = godotenv.Load()
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	switch cmd {
	case "up", "down":
		runMigrate(cmd, databaseURL)
	case "seed":
		runSeed(databaseURL, os.Args[2:])
	default:
		log.Fatalf("unknown command %q (want up|down|seed)", cmd)
	}
}

func runMigrate(cmd, databaseURL string) {
	m, err := migrate.New("file://migrations", databaseURL)
	if err != nil {
		log.Fatalf("init migrate: %v", err)
	}
	defer func() {
		srcErr, dbErr := m.Close()
		if srcErr != nil {
			log.Printf("close source: %v", srcErr)
		}
		if dbErr != nil {
			log.Printf("close db: %v", dbErr)
		}
	}()

	switch cmd {
	case "up":
		err = m.Up()
	case "down":
		err = m.Down()
	}

	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		log.Fatalf("migrate %s: %v", cmd, err)
	}
	fmt.Printf("migrate %s: ok\n", cmd)
}

func runSeed(databaseURL string, args []string) {
	fs := flag.NewFlagSet("seed", flag.ExitOnError)
	executor := fs.String("executor", "", "operator identity recorded in seed_history.executor_id (required)")
	if err := fs.Parse(args); err != nil {
		log.Fatalf("parse seed flags: %v", err)
	}
	if *executor == "" {
		log.Fatal("seed: -executor is required")
	}

	db, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
	if err != nil {
		log.Fatalf("connect to postgres: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("get underlying sql.DB: %v", err)
	}
	defer sqlDB.Close()

	artifact, checksum, err := seedsvc.LoadArtifactV1()
	if err != nil {
		log.Fatalf("load artifact: %v", err)
	}

	seeder := seedsvc.NewSeeder(seedrepo.NewRepository(db), idgen.New())
	result, err := seeder.Apply(context.Background(), artifact, checksum, *executor)
	if err != nil {
		log.Fatalf("seed: %v", err)
	}

	if !result.Applied {
		fmt.Printf("seed: no-op, version %s already applied\n", result.Version)
		return
	}
	fmt.Printf("seed: applied version %s (services created: %d, professionals created: %d, qualifications created: %d)\n",
		result.Version, result.ServicesCreated, result.ProfessionalsCreated, result.QualificationsCreated)
}
