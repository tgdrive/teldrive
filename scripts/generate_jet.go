//go:build ignore

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tgdrive/teldrive/internal/database"
	"github.com/tgdrive/teldrive/internal/database/jet"
)

const (
	postgresImage    = "ghcr.io/tgdrive/postgres:18"
	containerName    = "teldrive-test-postgres"
	postgresUser     = "teldrive_test"
	postgresPassword = "teldrive_test"
	postgresPort     = "55432"
	postgresDB       = "teldrive_test"
	jetDB            = "teldrive_jet"
	healthTimeout    = 60 * time.Second
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	rootDir, err := repoRoot()
	if err != nil {
		return err
	}

	defer cleanup()

	if err := removeContainer(); err != nil {
		return err
	}

	if err := startPostgres(); err != nil {
		return err
	}

	if err := waitForHealthyContainer(); err != nil {
		return err
	}

	if err := ensureJetDatabase(); err != nil {
		return err
	}

	dbURL := fmt.Sprintf(
		"postgres://%s:%s@localhost:%s/%s?sslmode=disable",
		postgresUser,
		postgresPassword,
		postgresPort,
		jetDB,
	)

	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		return fmt.Errorf("connect to postgres: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(context.Background()); err != nil {
		return fmt.Errorf("ping postgres: %w", err)
	}

	if err := database.MigrateDB(pool, false); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	jetGenDir := filepath.Join(rootDir, "internal", "database", "jet", "gen")
	if err := jet.Generate(dbURL, jetGenDir); err != nil {
		return fmt.Errorf("generate jet code: %w", err)
	}

	return nil
}

func repoRoot() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("resolve script path: runtime caller unavailable")
	}
	return filepath.Dir(filepath.Dir(file)), nil
}

func cleanup() {
	_ = removeContainer()
}

func removeContainer() error {
	return runQuietAllowFailure("docker", "rm", "-f", containerName)
}

func startPostgres() error {
	return runStreaming(
		"docker",
		"run",
		"-d",
		"--name",
		containerName,
		"-p",
		postgresPort+":5432",
		"-e",
		"POSTGRES_USER="+postgresUser,
		"-e",
		"POSTGRES_PASSWORD="+postgresPassword,
		"-e",
		"POSTGRES_DB="+postgresDB,
		"--health-cmd",
		fmt.Sprintf("pg_isready -U %s -d %s", postgresUser, postgresDB),
		"--health-interval",
		"2s",
		"--health-timeout",
		"2s",
		"--health-retries",
		"30",
		postgresImage,
	)
}

func waitForHealthyContainer() error {
	deadline := time.Now().Add(healthTimeout)
	for time.Now().Before(deadline) {
		status, err := inspectHealth()
		if err == nil && status == "healthy" {
			return nil
		}
		time.Sleep(time.Second)
	}

	status, err := inspectHealth()
	if err != nil {
		return fmt.Errorf("postgres container did not become healthy: %w", err)
	}
	return fmt.Errorf("postgres container is not healthy: %s", status)
}

func inspectHealth() (string, error) {
	output, err := runOutput(
		"docker",
		"inspect",
		"--format={{if .State.Health}}{{.State.Health.Status}}{{else}}unknown{{end}}",
		containerName,
	)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

func ensureJetDatabase() error {
	query := fmt.Sprintf("SELECT 1 FROM pg_database WHERE datname = '%s'", jetDB)
	output, err := runOutput(
		"docker",
		"exec",
		containerName,
		"psql",
		"-U",
		postgresUser,
		"-d",
		"postgres",
		"-Atqc",
		query,
	)
	if err != nil {
		return err
	}
	if strings.TrimSpace(output) == "1" {
		return nil
	}

	return runStreaming(
		"docker",
		"exec",
		containerName,
		"psql",
		"-U",
		postgresUser,
		"-d",
		"postgres",
		"-c",
		fmt.Sprintf("CREATE DATABASE \"%s\"", jetDB),
	)
}

func runQuietAllowFailure(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(output), "No such container: "+containerName) {
			return nil
		}
		return fmt.Errorf("%s %s: %w\n%s", name, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func runOutput(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w\n%s", name, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func runStreaming(name string, args ...string) error {
	return runStreamingInDirWithEnv("", nil, name, args...)
}

func runStreamingInDirWithEnv(dir string, env []string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}
