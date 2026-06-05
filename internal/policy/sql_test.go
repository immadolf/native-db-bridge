package policy

import "testing"

func TestClassifySQLQuery(t *testing.T) {
	cases := []struct {
		name string
		sql  string
		read bool
	}{
		{"select", "SELECT * FROM t", true},
		{"show", "SHOW TABLES", true},
		{"desc", "DESC t", true},
		{"describe", "DESCRIBE t", true},
		{"explain", "EXPLAIN SELECT * FROM t", true},
		{"update", "UPDATE t SET a=1", false},
		{"multi", "SELECT 1; DROP TABLE t", false},
		{"for update", "SELECT * FROM t FOR UPDATE", false},
		{"lock in share mode", "SELECT * FROM t LOCK IN SHARE MODE", false},
		{"load file", "SELECT LOAD_FILE('/etc/passwd')", false},
		{"into outfile", "SELECT * FROM t INTO OUTFILE '/tmp/x'", false},
		{"into dumpfile", "SELECT * FROM t INTO DUMPFILE '/tmp/x'", false},
		{"use", "USE other_db", false},
		{"lock", "LOCK TABLES t WRITE", false},
		{"unlock", "UNLOCK TABLES", false},
		{"call", "CALL p()", false},
		{"set", "SET @a=1", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsSQLReadAllowed(tc.sql)
			if got != tc.read {
				t.Fatalf("IsSQLReadAllowed=%v, want %v", got, tc.read)
			}
		})
	}
}

func TestIsSQLWriteAllowed(t *testing.T) {
	cases := []struct {
		name string
		sql  string
		kind string
		ok   bool
	}{
		{"insert", "INSERT INTO t (a) VALUES (1)", "insert", true},
		{"update", "UPDATE t SET a=1", "update", true},
		{"delete", "DELETE FROM t WHERE id=1", "delete", true},
		{"create", "CREATE TABLE t (id INT)", "ddl", true},
		{"drop", "DROP TABLE t", "ddl", true},
		{"alter", "ALTER TABLE t ADD COLUMN c INT", "ddl", true},
		{"select rejected", "SELECT * FROM t", "", false},
		{"show rejected", "SHOW TABLES", "", false},
		{"multi rejected", "INSERT INTO t VALUES (1); DROP TABLE x", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			kind, ok := IsSQLWriteAllowed(tc.sql)
			if ok != tc.ok || kind != tc.kind {
				t.Fatalf("IsSQLWriteAllowed=%q,%v, want %q,%v", kind, ok, tc.kind, tc.ok)
			}
		})
	}
}

func TestIsMultiStatement(t *testing.T) {
	cases := []struct {
		name     string
		sql      string
		multi    bool
	}{
		{"single", "SELECT * FROM t", false},
		{"multi", "SELECT 1; DROP TABLE t", true},
		{"semicolon in single quote", "SELECT 'a;b'", false},
		{"semicolon in double quote", `SELECT "a;b"`, false},
		{"trailing semicolon", "SELECT * FROM t;", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isMultiStatement(tc.sql)
			if got != tc.multi {
				t.Fatalf("isMultiStatement(%q)=%v, want %v", tc.sql, got, tc.multi)
			}
		})
	}
}
