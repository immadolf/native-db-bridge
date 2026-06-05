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
