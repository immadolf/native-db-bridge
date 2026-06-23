package tools

import (
	"slices"
	"testing"
)

func TestToolSchemasIncludeRequiredTools(t *testing.T) {
	names := ToolNames()
	want := []string{
		"datasource_list", "datasource_healthcheck",
		"sql_schema_list", "sql_object_type_list", "sql_object_list", "sql_object_describe", "sql_table_preview",
		"sql_column_search", "sql_text_column_plan",
		"redis_key_scan", "redis_key_describe",
		"mongo_database_list", "mongo_collection_list", "mongo_collection_describe",
		"sql_query", "sql_text_scan", "sql_prepare_change", "redis_command", "redis_prepare_change", "mongo_find", "mongo_prepare_change", "execute_confirmation",
		"operation_list", "cancel_operation", "audit_recent", "audit_summary", "confirmation_get", "cancel_confirmation",
	}
	for _, w := range want {
		if !slices.Contains(names, w) {
			t.Fatalf("missing tool %s", w)
		}
	}
}

func TestToolNamesCount(t *testing.T) {
	names := ToolNames()
	if len(names) != 28 {
		t.Fatalf("expected 28 tools, got %d", len(names))
	}
}

func TestToolNamesIncludesSQLColumnSearch(t *testing.T) {
	names := ToolNames()
	if !slices.Contains(names, "sql_column_search") {
		t.Fatal("ToolNames should include sql_column_search")
	}
}

func TestToolNamesNoDuplicates(t *testing.T) {
	names := ToolNames()
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		if seen[name] {
			t.Fatalf("duplicate tool name: %s", name)
		}
		seen[name] = true
	}
}
