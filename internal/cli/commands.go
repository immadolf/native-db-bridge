package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"native-db-bridge-mcp/internal/app"
	"native-db-bridge-mcp/internal/backend"
	"native-db-bridge-mcp/internal/config"
)

// CommandNames returns the list of available CLI commands.
func CommandNames() []string {
	return []string{"serve", "healthcheck", "install-service", "uninstall-service"}
}

// resolveHome determines the home directory from the flag, environment,
// or default in that priority order.
func resolveHome(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	if env := os.Getenv("NATIVE_DB_BRIDGE_HOME"); env != "" {
		return env
	}
	return "./var"
}

// Dispatch routes os.Args[1] to the appropriate command handler and
// returns the exit code (0 for success).
func Dispatch(args []string) int {
	if len(args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: native-db-bridge-mcp <%v>\n", CommandNames())
		return 2
	}

	cmd := args[1]
	cmdArgs := args[2:]

	switch cmd {
	case "serve":
		if err := Serve(cmdArgs); err != nil {
			fmt.Fprintf(os.Stderr, "serve: %v\n", err)
			return 1
		}
		return 0
	case "healthcheck":
		if err := Healthcheck(cmdArgs); err != nil {
			fmt.Fprintf(os.Stderr, "healthcheck: %v\n", err)
			return 1
		}
		return 0
	case "install-service":
		if err := InstallServiceCmd(cmdArgs); err != nil {
			fmt.Fprintf(os.Stderr, "install-service: %v\n", err)
			return 1
		}
		return 0
	case "uninstall-service":
		if err := UninstallService(); err != nil {
			fmt.Fprintf(os.Stderr, "uninstall-service: %v\n", err)
			return 1
		}
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", cmd)
		return 2
	}
}

// Serve loads config from <home>/config.yaml, creates the App, and
// starts the HTTP server. It blocks until SIGINT or SIGTERM.
func Serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	home := fs.String("home", "", "home directory (default: $NATIVE_DB_BRIDGE_HOME or ./var)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	homeDir := resolveHome(*home)
	cfgPath := filepath.Join(homeDir, "config.yaml")

	a, err := app.New(cfgPath)
	if err != nil {
		return fmt.Errorf("failed to create app: %w", err)
	}
	defer a.Close()

	// Graceful shutdown on SIGINT/SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		errCh <- a.Server().ListenAndServe()
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		fmt.Fprintln(os.Stderr, "shutting down...")
		return nil
	}
}

// Healthcheck validates the config and optionally tests real connections.
func Healthcheck(args []string) error {
	fs := flag.NewFlagSet("healthcheck", flag.ContinueOnError)
	home := fs.String("home", "", "home directory (default: $NATIVE_DB_BRIDGE_HOME or ./var)")
	connect := fs.Bool("connect", false, "test real backend connections")
	if err := fs.Parse(args); err != nil {
		return err
	}

	homeDir := resolveHome(*home)
	cfgPath := filepath.Join(homeDir, "config.yaml")

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	fmt.Println("config OK")

	if !*connect {
		return nil
	}

	return healthcheckConnect(cfg)
}

// InstallServiceCmd parses flags for install-service and calls InstallService.
func InstallServiceCmd(args []string) error {
	fs := flag.NewFlagSet("install-service", flag.ContinueOnError)
	home := fs.String("home", "", "home directory (default: $NATIVE_DB_BRIDGE_HOME or ./var)")
	bin := fs.String("bin", "", "path to the native-db-bridge-mcp binary")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *bin == "" {
		return fmt.Errorf("--bin flag is required")
	}

	homeDir := resolveHome(*home)
	return InstallService(homeDir, *bin)
}

// healthcheckConnect pings all configured datasources.
func healthcheckConnect(cfg *config.Config) error {
	var firstErr error

	setFirst := func(label string, err error) {
		if err != nil {
			fmt.Fprintf(os.Stderr, "  %s: FAIL: %v\n", label, err)
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", label, err)
			}
		} else {
			fmt.Printf("  %s: OK\n", label)
		}
	}

	ctx := context.Background()

	// SQL datasources.
	sqlBackend := backend.NewSQLDriverBackend(*cfg)
	defer sqlBackend.Close()
	for _, ds := range cfg.Datasources.SQL {
		err := sqlBackend.Ping(ctx, ds.Name)
		setFirst("sql/"+ds.Name, err)
	}

	// Redis datasources.
	redisBackend := backend.NewRedisDriverBackend(*cfg)
	defer redisBackend.Close()
	for _, ds := range cfg.Datasources.Redis {
		err := redisBackend.Ping(ctx, ds.Name)
		setFirst("redis/"+ds.Name, err)
	}

	// Mongo datasources.
	mongoBackend := backend.NewMongoDriverBackend(*cfg)
	defer mongoBackend.Close()
	for _, ds := range cfg.Datasources.Mongo {
		err := mongoBackend.Ping(ctx, ds.Name)
		setFirst("mongo/"+ds.Name, err)
	}

	if firstErr != nil {
		return fmt.Errorf("one or more connections failed")
	}
	fmt.Println("all connections OK")
	return nil
}
