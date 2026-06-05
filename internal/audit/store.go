package audit

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"

	_ "modernc.org/sqlite"
)

// Confirmation represents a pending or completed confirmation request.
type Confirmation struct {
	ID           string
	Kind         string
	Datasource   string
	PayloadJSON  string
	PayloadHash  string
	Summary      string
	RiskLevel    string
	ImpactJSON   string
	Status       string
	ExpiresAt    time.Time
	ExecutedAt   *time.Time
	ErrorSummary *string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Store provides access to the SQLite-backed audit database.
type Store struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at path, enforces 0600
// permissions, runs an integrity check, enables WAL mode, and applies
// migrations.
func Open(path string) (*Store, error) {
	// Ensure the file exists with correct permissions before sql.Open.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("audit: create db file: %w", err)
	}
	f.Close()

	// Reject existing files with group/other bits set.
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("audit: stat db file: %w", err)
	}
	perm := info.Mode().Perm()
	if perm&0077 != 0 {
		return nil, fmt.Errorf("audit: db file %s has mode %#o, want 0600", path, perm)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("audit: open db: %w", err)
	}

	// Enforce 0600 after open in case SQLite recreated the file.
	if err := os.Chmod(path, 0600); err != nil {
		db.Close()
		return nil, fmt.Errorf("audit: chmod db file: %w", err)
	}

	s := &Store{db: db}

	if err := s.bootstrap(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// bootstrap runs integrity check, enables WAL, and applies migrations.
func (s *Store) bootstrap() error {
	var result string
	if err := s.db.QueryRow("PRAGMA integrity_check").Scan(&result); err != nil {
		return fmt.Errorf("audit: integrity_check: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("audit: integrity_check returned %q", result)
	}

	if _, err := s.db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return fmt.Errorf("audit: enable WAL: %w", err)
	}

	if _, err := s.db.Exec(migrationSQL); err != nil {
		return fmt.Errorf("audit: migration: %w", err)
	}
	return nil
}

// CheckSchema verifies that all expected tables exist.
func (s *Store) CheckSchema() error {
	tables := []string{"confirmations", "audit_events", "operations"}
	for _, tbl := range tables {
		var name string
		err := s.db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", tbl,
		).Scan(&name)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("audit: table %q missing", tbl)
		}
		if err != nil {
			return fmt.Errorf("audit: check table %q: %w", tbl, err)
		}
	}
	return nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// CreateConfirmation inserts a new confirmation record.
func (s *Store) CreateConfirmation(conf Confirmation) error {
	now := time.Now()
	_, err := s.db.Exec(`
		INSERT INTO confirmations
			(id, kind, datasource, payload_json, payload_hash, summary,
			 risk_level, impact_json, status, expires_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		conf.ID, conf.Kind, conf.Datasource, conf.PayloadJSON, conf.PayloadHash,
		conf.Summary, conf.RiskLevel, conf.ImpactJSON, conf.Status,
		conf.ExpiresAt, now, now,
	)
	if err != nil {
		return fmt.Errorf("audit: create confirmation %q: %w", conf.ID, err)
	}
	return nil
}

// GetConfirmation retrieves a confirmation by ID.
func (s *Store) GetConfirmation(id string) (*Confirmation, error) {
	var c Confirmation
	err := s.db.QueryRow(`
		SELECT id, kind, datasource, payload_json, payload_hash, summary,
		       risk_level, impact_json, status, expires_at, executed_at,
		       error_summary, created_at, updated_at
		FROM confirmations WHERE id=?`, id,
	).Scan(
		&c.ID, &c.Kind, &c.Datasource, &c.PayloadJSON, &c.PayloadHash,
		&c.Summary, &c.RiskLevel, &c.ImpactJSON, &c.Status, &c.ExpiresAt,
		&c.ExecutedAt, &c.ErrorSummary, &c.CreatedAt, &c.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("audit: confirmation %q not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("audit: get confirmation %q: %w", id, err)
	}
	return &c, nil
}

// MarkConfirmationExecuting atomically transitions a pending, non-expired
// confirmation to "executing". Only one concurrent caller succeeds.
func (s *Store) MarkConfirmationExecuting(id string) error {
	now := time.Now()
	result, err := s.db.Exec(`
		UPDATE confirmations
		SET status='executing', updated_at=?
		WHERE id=? AND status='pending' AND expires_at > ?`,
		now, id, now,
	)
	if err != nil {
		return fmt.Errorf("audit: mark executing %q: %w", id, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("audit: mark executing %q rows affected: %w", id, err)
	}
	if rows == 0 {
		return fmt.Errorf("audit: confirmation %q not in pending state or expired", id)
	}
	return nil
}

// MarkConfirmationExecuted transitions a confirmation to "executed".
func (s *Store) MarkConfirmationExecuted(id string) error {
	now := time.Now()
	result, err := s.db.Exec(`
		UPDATE confirmations
		SET status='executed', executed_at=?, updated_at=?
		WHERE id=? AND status='executing'`,
		now, now, id,
	)
	if err != nil {
		return fmt.Errorf("audit: mark executed %q: %w", id, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("audit: mark executed %q rows affected: %w", id, err)
	}
	if rows == 0 {
		return fmt.Errorf("audit: confirmation %q not in executing state", id)
	}
	return nil
}

// MarkConfirmationFailed transitions a confirmation to "failed".
func (s *Store) MarkConfirmationFailed(id string, errSummary string) error {
	now := time.Now()
	result, err := s.db.Exec(`
		UPDATE confirmations
		SET status='failed', error_summary=?, updated_at=?
		WHERE id=? AND status='executing'`,
		errSummary, now, id,
	)
	if err != nil {
		return fmt.Errorf("audit: mark failed %q: %w", id, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("audit: mark failed %q rows affected: %w", id, err)
	}
	if rows == 0 {
		return fmt.Errorf("audit: confirmation %q not in executing state", id)
	}
	return nil
}

// MarkConfirmationExpired transitions a confirmation to "expired".
func (s *Store) MarkConfirmationExpired(id string) error {
	now := time.Now()
	result, err := s.db.Exec(`
		UPDATE confirmations
		SET status='expired', updated_at=?
		WHERE id=? AND status IN ('pending', 'executing')`,
		now, id,
	)
	if err != nil {
		return fmt.Errorf("audit: mark expired %q: %w", id, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("audit: mark expired %q rows affected: %w", id, err)
	}
	if rows == 0 {
		return fmt.Errorf("audit: confirmation %q not in a valid state for expiry", id)
	}
	return nil
}

// MarkConfirmationCancelled transitions a confirmation to "cancelled".
func (s *Store) MarkConfirmationCancelled(id string) error {
	now := time.Now()
	result, err := s.db.Exec(`
		UPDATE confirmations
		SET status='cancelled', updated_at=?
		WHERE id=? AND status IN ('pending', 'executing')`,
		now, id,
	)
	if err != nil {
		return fmt.Errorf("audit: mark cancelled %q: %w", id, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("audit: mark cancelled %q rows affected: %w", id, err)
	}
	if rows == 0 {
		return fmt.Errorf("audit: confirmation %q not in a valid state for cancellation", id)
	}
	return nil
}
