package audit

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenCreatesDatabaseAndMigrates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.db")
	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error=%v", err)
	}
	defer store.Close()
	if err := store.CheckSchema(); err != nil {
		t.Fatalf("CheckSchema() error=%v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("audit.db mode=%#o, want 0600", got)
	}
}

func TestConfirmationLifecycle(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	conf := Confirmation{
		ID:          "conf_test",
		Kind:        "sql_dml",
		Datasource:  "saas_support",
		PayloadJSON: `{"sql":"UPDATE t SET a=1 WHERE id=1"}`,
		PayloadHash: "hash",
		Summary:     "UPDATE t ...",
		RiskLevel:   "medium",
		ImpactJSON:  `{"mode":"estimated","rows":1}`,
		Status:      "pending",
		ExpiresAt:   time.Now().Add(time.Minute),
	}
	if err := store.CreateConfirmation(conf); err != nil {
		t.Fatalf("CreateConfirmation() error=%v", err)
	}
	got, err := store.GetConfirmation("conf_test")
	if err != nil {
		t.Fatalf("GetConfirmation() error=%v", err)
	}
	if got.Summary != conf.Summary {
		t.Fatalf("summary=%q", got.Summary)
	}
}

func TestMarkExpiredConfirmations(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Create an expired pending confirmation (expires in the past).
	expiredConf := Confirmation{
		ID:          "conf_expired",
		Kind:        "sql_dml",
		Datasource:  "saas_support",
		PayloadJSON: `{"sql":"UPDATE t SET a=1"}`,
		PayloadHash: "hash1",
		Summary:     "expired update",
		RiskLevel:   "low",
		ImpactJSON:  `{"mode":"estimated","rows":1}`,
		Status:      "pending",
		ExpiresAt:   time.Now().Add(-time.Minute), // already expired
	}
	if err := store.CreateConfirmation(expiredConf); err != nil {
		t.Fatal(err)
	}

	// Create a still-valid pending confirmation (expires in the future).
	validConf := Confirmation{
		ID:          "conf_valid",
		Kind:        "sql_dml",
		Datasource:  "saas_support",
		PayloadJSON: `{"sql":"UPDATE t SET a=2"}`,
		PayloadHash: "hash2",
		Summary:     "valid update",
		RiskLevel:   "low",
		ImpactJSON:  `{"mode":"estimated","rows":1}`,
		Status:      "pending",
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	if err := store.CreateConfirmation(validConf); err != nil {
		t.Fatal(err)
	}

	// Run the expiry scanner.
	now := time.Now()
	affected, err := store.MarkExpiredConfirmations(now)
	if err != nil {
		t.Fatalf("MarkExpiredConfirmations() error=%v", err)
	}
	if affected != 1 {
		t.Fatalf("MarkExpiredConfirmations() affected=%d, want 1", affected)
	}

	// Verify the expired confirmation was transitioned.
	got, err := store.GetConfirmation("conf_expired")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "expired" {
		t.Fatalf("expired conf status=%q, want \"expired\"", got.Status)
	}

	// Verify the valid confirmation is still pending.
	got2, err := store.GetConfirmation("conf_valid")
	if err != nil {
		t.Fatal(err)
	}
	if got2.Status != "pending" {
		t.Fatalf("valid conf status=%q, want \"pending\"", got2.Status)
	}

	// Running again should affect zero rows.
	affected2, err := store.MarkExpiredConfirmations(now)
	if err != nil {
		t.Fatalf("MarkExpiredConfirmations() second call error=%v", err)
	}
	if affected2 != 0 {
		t.Fatalf("MarkExpiredConfirmations() second call affected=%d, want 0", affected2)
	}
}

func TestExecuteConfirmationCanWinOnlyOnce(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	conf := Confirmation{
		ID:          "conf_race",
		Kind:        "sql_dml",
		Datasource:  "saas_support",
		PayloadJSON: `{"sql":"UPDATE t SET a=1 WHERE id=1"}`,
		PayloadHash: "hash",
		Summary:     "UPDATE t ...",
		RiskLevel:   "medium",
		ImpactJSON:  `{"mode":"estimated","rows":1}`,
		Status:      "pending",
		ExpiresAt:   time.Now().Add(time.Minute),
	}
	if err := store.CreateConfirmation(conf); err != nil {
		t.Fatal(err)
	}
	const workers = 8
	results := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func() {
			results <- store.MarkConfirmationExecuting("conf_race")
		}()
	}
	success := 0
	for i := 0; i < workers; i++ {
		if err := <-results; err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("success=%d, want 1", success)
	}
}

func TestExecutingConfirmationCannotExpireOrCancel(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	conf := Confirmation{
		ID:          "conf_executing",
		Kind:        "sql_dml",
		Datasource:  "saas_support",
		PayloadJSON: `{"sql":"UPDATE t SET a=1 WHERE id=1"}`,
		PayloadHash: "hash",
		Summary:     "UPDATE t ...",
		RiskLevel:   "medium",
		ImpactJSON:  `{"mode":"estimated","rows":1}`,
		Status:      "pending",
		ExpiresAt:   time.Now().Add(time.Minute),
	}
	if err := store.CreateConfirmation(conf); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkConfirmationExecuting(conf.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkConfirmationExpired(conf.ID); err == nil {
		t.Fatal("executing confirmation must not expire")
	}
	if err := store.MarkConfirmationCancelled(conf.ID); err == nil {
		t.Fatal("executing confirmation must not cancel")
	}
	got, err := store.GetConfirmation(conf.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "executing" {
		t.Fatalf("status=%q, want executing", got.Status)
	}
}

func TestOperationStatusTransitionsPersist(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	op := Operation{
		ID:         "op_running",
		Kind:       "sql_query",
		Datasource: "saas_support",
		Status:     "running",
		StartedAt:  time.Now(),
	}
	if err := store.InsertOperation(op); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkOperationCancelRequested(op.ID); err != nil {
		t.Fatal(err)
	}
	cancelled, err := store.ListOperations("cancel_requested", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(cancelled) != 1 || cancelled[0].CancelRequestedAt == nil {
		t.Fatalf("cancel_requested operation not persisted: %+v", cancelled)
	}
	if err := store.MarkOperationFinished(op.ID, "DRIVER_ERROR", "cancelled"); err != nil {
		t.Fatal(err)
	}
	finished, err := store.ListOperations("finished", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(finished) != 1 || finished[0].FinishedAt == nil {
		t.Fatalf("finished operation not persisted: %+v", finished)
	}
	if finished[0].ErrorCode != "DRIVER_ERROR" || finished[0].ErrorSummary != "cancelled" {
		t.Fatalf("operation error fields not persisted: %+v", finished[0])
	}
}

func TestAuditSummaryByErrorCode(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now()
	err = store.InsertAuditEvent(AuditEvent{
		ID:         "evt_1",
		EventType:  "sql_query",
		Datasource: "saas_support",
		Summary:    "SELECT missing_col FROM users",
		Status:     "error",
		ErrorCode:  "SQL_UNKNOWN_COLUMN",
		CreatedAt:  now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateConfirmation(Confirmation{
		ID:         "conf_1",
		Kind:       "sql_dml",
		Datasource: "saas_support",
		Summary:    "UPDATE users SET name = ? WHERE id = ?",
		RiskLevel:  "medium",
		ImpactJSON: "{}",
		Status:     "pending",
		ExpiresAt:  now.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkConfirmationExecuting("conf_1"); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkConfirmationExecuted("conf_1"); err != nil {
		t.Fatal(err)
	}

	rows, err := store.Summary(SummaryFilter{
		StartTime:  now.Add(-time.Hour),
		EndTime:    now.Add(time.Hour),
		Datasource: "saas_support",
		GroupBy:    "error_code",
		Limit:      10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows=%#v", rows)
	}
	if rows[0].ErrorCode != "SQL_UNKNOWN_COLUMN" || rows[0].Count != 1 {
		t.Fatalf("row=%#v", rows[0])
	}
	topErrors, err := store.TopErrorSummaries(SummaryFilter{
		StartTime:  now.Add(-time.Hour),
		EndTime:    now.Add(time.Hour),
		Datasource: "saas_support",
		Limit:      10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(topErrors) != 1 || topErrors[0].Summary != "SELECT missing_col FROM users" {
		t.Fatalf("topErrors=%#v", topErrors)
	}
	confirmations, err := store.ConfirmationSummary(SummaryFilter{
		StartTime:  now.Add(-time.Hour),
		EndTime:    now.Add(time.Hour),
		Datasource: "saas_support",
		Limit:      10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(confirmations) != 1 || confirmations[0].Status != "executed" || confirmations[0].Count != 1 {
		t.Fatalf("confirmations=%#v", confirmations)
	}
}
