package nbderrors

import "testing"

func TestErrorShape(t *testing.T) {
	err := New(CodePolicyRedisSelectRejected, "Redis SELECT is rejected").WithDatasource("saas-auth-support")
	if err.Code != CodePolicyRedisSelectRejected {
		t.Fatalf("code=%s", err.Code)
	}
	if err.Category != CategoryPolicy {
		t.Fatalf("category=%s", err.Category)
	}
	if err.Datasource != "saas-auth-support" {
		t.Fatalf("datasource=%s", err.Datasource)
	}
	if err.Retryable {
		t.Fatalf("policy errors must not be retryable")
	}
}

func TestClassifySQLError(t *testing.T) {
	tests := []struct {
		name      string
		message   string
		wantCode  Code
		retryable bool
	}{
		{
			name:      "unknown column",
			message:   "sql query saas_support: Error 1054 (42S22): Unknown column 'remark' in 'field list'",
			wantCode:  CodeSQLUnknownColumn,
			retryable: false,
		},
		{
			name:      "unknown table",
			message:   "sql query saas_support: Error 1146 (42S02): Table 'saas_support.tch_x' doesn't exist",
			wantCode:  CodeSQLUnknownTable,
			retryable: false,
		},
		{
			name:      "syntax",
			message:   "sql query saas_support: Error 1064 (42000): You have an error in your SQL syntax",
			wantCode:  CodeQuerySyntaxError,
			retryable: false,
		},
		{
			name:      "timeout",
			message:   "sql query saas_support: context deadline exceeded",
			wantCode:  CodeQueryTimeout,
			retryable: true,
		},
		{
			name:      "connection reset",
			message:   "sql query saas_support: connection reset by peer",
			wantCode:  CodeConnectionFailed,
			retryable: true,
		},
		{
			name:      "other driver",
			message:   "sql query saas_support: unexpected driver failure",
			wantCode:  CodeDriverError,
			retryable: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ClassifySQLErrorMessage(tt.message)
			if err.Code != tt.wantCode {
				t.Fatalf("code=%s, want %s", err.Code, tt.wantCode)
			}
			if err.Retryable != tt.retryable {
				t.Fatalf("retryable=%v, want %v", err.Retryable, tt.retryable)
			}
		})
	}
}
