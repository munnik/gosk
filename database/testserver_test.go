package database_test

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// testServer is a throwaway PostgreSQL/TimescaleDB instance that the suite
// creates and destroys itself: initdb into a temporary directory, start it
// on a unix socket, create the test database, and delete the whole thing
// afterwards.
//
// The suite used to expect a database already running on localhost:5432,
// which meant it could not run unattended - in particular not in the Nix
// build sandbox, which has no network, no service manager and no state
// that outlives the build. Everything here stays inside one temporary
// directory and needs nothing but initdb/pg_ctl on PATH.
type testServer struct {
	baseDir string
	dataDir string
	sockDir string
	logFile string
}

// startTestServer initialises and starts the server. The caller owns it and
// must call stop, even when start itself fails partway.
func startTestServer() (*testServer, error) {
	initdb, err := exec.LookPath("initdb")
	if err != nil {
		return nil, fmt.Errorf("initdb not found on PATH, the database suite needs postgresql with the timescaledb extension available: %w", err)
	}
	pgCtl, err := exec.LookPath("pg_ctl")
	if err != nil {
		return nil, fmt.Errorf("pg_ctl not found on PATH, the database suite needs postgresql with the timescaledb extension available: %w", err)
	}

	// Deliberately under TMPDIR rather than the working directory: a unix
	// socket path has a hard limit of ~100 characters, and TMPDIR is short
	// both in the Nix sandbox (/build) and on a developer machine (/tmp).
	baseDir, err := os.MkdirTemp("", "gosk-pgtest-")
	if err != nil {
		return nil, err
	}
	s := &testServer{
		baseDir: baseDir,
		dataDir: filepath.Join(baseDir, "data"),
		sockDir: filepath.Join(baseDir, "sock"),
		logFile: filepath.Join(baseDir, "postgres.log"),
	}
	if err := os.Mkdir(s.sockDir, 0o700); err != nil {
		return nil, err
	}

	// --locale=C and an explicit encoding because the Nix sandbox has no
	// locale archive, and initdb inherits a locale it cannot resolve.
	// --auth=trust and --no-sync are safe here and save several seconds:
	// nothing but this process can reach the socket, and the data
	// directory is deleted at the end of the suite either way.
	if out, err := exec.Command(
		initdb,
		"--pgdata="+s.dataDir,
		"--username=postgres",
		"--auth=trust",
		"--encoding=UTF8",
		"--locale=C",
		"--no-sync",
	).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("initdb failed: %w\n%s", err, out)
	}

	if err := s.writeConfig(); err != nil {
		return nil, err
	}

	if out, err := exec.Command(
		pgCtl,
		"--pgdata="+s.dataDir,
		"--log="+s.logFile,
		"--wait",
		"start",
	).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("pg_ctl start failed: %w\n%s\n%s", err, out, s.log())
	}

	return s, nil
}

// writeConfig appends the settings the suite needs to the configuration
// initdb generated.
func (s *testServer) writeConfig() error {
	settings := strings.Join([]string{
		// timescaledb has to be preloaded before CREATE EXTENSION in the
		// first migration can work at all.
		"shared_preload_libraries = 'timescaledb'",
		// Telemetry phones home on a timer; the sandbox has no network and
		// the tests have no business reporting anything.
		"timescaledb.telemetry_level = 'off'",
		// Continuous aggregate policies (20230203140028 onwards) run as
		// background jobs, and timescaledb warns and degrades if the
		// server cannot spare workers for them.
		"max_worker_processes = 16",
		"timescaledb.max_background_workers = 8",
		// Unix socket only: no port to collide with a developer's own
		// postgres, and no network in the sandbox to listen on anyway.
		"listen_addresses = ''",
		fmt.Sprintf("unix_socket_directories = '%s'", s.sockDir),
		// The data directory does not outlive the suite, so durability is
		// pure cost here.
		"fsync = off",
		"full_page_writes = off",
		"synchronous_commit = off",
	}, "\n")

	f, err := os.OpenFile(filepath.Join(s.dataDir, "postgresql.conf"), os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "\n# gosk test suite\n%s\n", settings)
	return err
}

// url is the connection string for one database on this server.
func (s *testServer) url(database string) string {
	// The socket directory goes in the query string because it is a path,
	// not a host name. sslmode=disable because the server has no
	// certificate and golang-migrate's driver otherwise refuses the
	// connection outright with "pq: SSL is not enabled on the server".
	q := url.Values{}
	q.Set("host", s.sockDir)
	q.Set("sslmode", "disable")
	return "postgres://postgres@/" + database + "?" + q.Encode()
}

// createDatabase creates a database on the running server.
func (s *testServer) createDatabase(database string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, s.url("postgres"))
	if err != nil {
		return fmt.Errorf("could not connect to the test server: %w\n%s", err, s.log())
	}
	defer conn.Close(ctx)

	// The name is a constant in this package, never user input.
	if _, err := conn.Exec(ctx, `CREATE DATABASE "`+database+`"`); err != nil {
		return fmt.Errorf("could not create database %q: %w", database, err)
	}
	return nil
}

// stop shuts the server down and deletes everything it created. It is safe
// to call on a half-started server.
func (s *testServer) stop() error {
	var stopErr error
	if pgCtl, err := exec.LookPath("pg_ctl"); err == nil {
		// Immediate: nothing in here needs a clean shutdown, and a
		// checkpoint on the way out is wasted work on a directory that is
		// about to be deleted.
		if out, err := exec.Command(
			pgCtl,
			"--pgdata="+s.dataDir,
			"--mode=immediate",
			"--wait",
			"stop",
		).CombinedOutput(); err != nil {
			stopErr = fmt.Errorf("pg_ctl stop failed: %w\n%s", err, out)
		}
	}

	if err := os.RemoveAll(s.baseDir); err != nil && stopErr == nil {
		stopErr = err
	}
	return stopErr
}

// log returns the server log, to attach to a failure that the Go-level
// error alone does not explain.
func (s *testServer) log() string {
	contents, err := os.ReadFile(s.logFile)
	if err != nil {
		return ""
	}
	return "postgres log:\n" + string(contents)
}
