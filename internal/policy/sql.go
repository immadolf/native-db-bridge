package policy

import (
	"strings"

	"github.com/xwb1989/sqlparser"
)

// IsSQLReadAllowed reports whether the SQL statement is a safe read-only
// operation. It uses the sqlparser AST for classification -- no prefix
// matching. Multi-statements and dangerous patterns are always rejected.
func IsSQLReadAllowed(sql string) bool {
	if isMultiStatement(sql) {
		return false
	}

	stmt, err := sqlparser.Parse(sql)
	if err != nil {
		return false
	}

	switch s := stmt.(type) {
	case *sqlparser.Select:
		if isUnsafeLockMode(s.Lock) {
			return false
		}
		if containsDangerousSQLPatterns(sql) {
			return false
		}
		return true
	case *sqlparser.Union:
		if isUnsafeLockMode(s.Lock) {
			return false
		}
		if containsDangerousSQLPatterns(sql) {
			return false
		}
		return true
	case *sqlparser.Show:
		return true
	case *sqlparser.OtherRead:
		// DESCRIBE, EXPLAIN
		return true
	default:
		return false
	}
}

// IsSQLWriteAllowed reports whether the SQL statement is an acceptable write
// operation (INSERT, UPDATE, DELETE, or DDL). It returns the kind of write
// and true if allowed, or ("", false) if rejected.
func IsSQLWriteAllowed(sql string) (string, bool) {
	if isMultiStatement(sql) {
		return "", false
	}

	stmt, err := sqlparser.Parse(sql)
	if err != nil {
		return "", false
	}

	switch stmt.(type) {
	case *sqlparser.Insert:
		return "insert", true
	case *sqlparser.Update:
		return "update", true
	case *sqlparser.Delete:
		return "delete", true
	case *sqlparser.DDL:
		return "ddl", true
	default:
		return "", false
	}
}

// isMultiStatement detects whether the SQL contains more than one statement
// by scanning for an unquoted semicolon followed by non-whitespace content.
func isMultiStatement(sql string) bool {
	inSingleQuote := false
	inDoubleQuote := false

	for i := 0; i < len(sql); i++ {
		ch := sql[i]

		if ch == '\'' && !inDoubleQuote {
			if inSingleQuote {
				// Check for escaped quote ('')
				if i+1 < len(sql) && sql[i+1] == '\'' {
					i++ // skip escaped quote
					continue
				}
				inSingleQuote = false
			} else {
				inSingleQuote = true
			}
			continue
		}

		if ch == '"' && !inSingleQuote {
			if inDoubleQuote {
				if i+1 < len(sql) && sql[i+1] == '"' {
					i++
					continue
				}
				inDoubleQuote = false
			} else {
				inDoubleQuote = true
			}
			continue
		}

		if ch == ';' && !inSingleQuote && !inDoubleQuote {
			// Check if there is non-whitespace content after the semicolon
			remaining := strings.TrimSpace(sql[i+1:])
			if len(remaining) > 0 {
				return true
			}
		}
	}

	return false
}

// isUnsafeLockMode returns true if the SELECT lock clause indicates a
// write-semantic lock (FOR UPDATE or LOCK IN SHARE MODE).
func isUnsafeLockMode(lock string) bool {
	upper := strings.ToUpper(strings.TrimSpace(lock))
	return upper == "FOR UPDATE" || upper == "LOCK IN SHARE MODE" || upper == "FOR SHARE"
}

// containsDangerousSQLPatterns checks the raw SQL for patterns that indicate
// filesystem access or other dangerous operations even within a SELECT.
func containsDangerousSQLPatterns(sql string) bool {
	upper := strings.ToUpper(sql)
	dangerous := []string{
		"LOAD_FILE",
		"INTO OUTFILE",
		"INTO DUMPFILE",
	}
	for _, pattern := range dangerous {
		if strings.Contains(upper, pattern) {
			return true
		}
	}
	return false
}
