// Command bootstrap initializes the single ROOT identity of an installation.
// It never creates default credentials: the password is read interactively
// without echo, or from a single explicit stdin line for controlled automation.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/audit"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/auth"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/config"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/database"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/observability"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "nexora-bootstrap: "+err.Error())
		os.Exit(1)
	}
}

func run() error {
	username := flag.String("username", "waiting", "username for the ROOT account")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	logger := observability.NewLogger(cfg.LogLevel)

	db, err := database.OpenSQL(cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("connect to database: %w", database.SafeError(err))
	}
	version, err := database.Version(ctx, db)
	if err != nil {
		return err
	}
	if version < 2 {
		return fmt.Errorf("schema version %d does not include identity tables; run nexora-migrate -command up first", version)
	}

	pool, err := database.Open(ctx, cfg)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()

	password, err := readPassword()
	if err != nil {
		return err
	}
	normalized := strings.ToLower(strings.TrimSpace(*username))
	if msg := auth.UsernameFieldError(normalized); msg != "" {
		return fmt.Errorf("username %s", msg)
	}
	if msg := auth.PasswordFieldError(normalized, password); msg != "" {
		return fmt.Errorf("password %s", msg)
	}

	svc := auth.NewService(
		auth.NewRepository(pool),
		audit.NewRepository(pool),
		auth.NewLoginLimiter(cfg.LoginMaxFailures, cfg.LoginWindow),
		cfg.SessionTTL,
		logger,
	)
	user, err := svc.BootstrapRoot(ctx, normalized, password)
	switch {
	case errors.Is(err, auth.ErrRootAlreadyInitialized):
		return errors.New("ROOT is already initialized; refusing to bootstrap again")
	case errors.Is(err, auth.ErrUsernameTaken):
		return fmt.Errorf("username %q is already in use", normalized)
	case err != nil:
		return err
	}
	fmt.Printf("ROOT account %q initialized (id %s).\n", user.Username, user.ID)
	return nil
}

func readPassword() (string, error) {
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		fmt.Fprint(os.Stderr, "Set ROOT password: ")
		first, err := term.ReadPassword(fd)
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", fmt.Errorf("read password: %w", err)
		}
		fmt.Fprint(os.Stderr, "Confirm ROOT password: ")
		second, err := term.ReadPassword(fd)
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", fmt.Errorf("read password confirmation: %w", err)
		}
		if string(first) != string(second) {
			return "", errors.New("passwords do not match")
		}
		return string(first), nil
	}
	// Non-interactive automation still requires an explicit password line;
	// there is no default and nothing is echoed back.
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return "", errors.New("no password provided on stdin")
	}
	password := strings.TrimRight(line, "\r\n")
	if password == "" {
		return "", errors.New("no password provided on stdin")
	}
	return password, nil
}
