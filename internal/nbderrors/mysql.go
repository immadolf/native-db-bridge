package nbderrors

import "strings"

// ClassifySQLErrorMessage maps common MySQL and driver failures to actionable
// native-db-bridge error codes.
func ClassifySQLErrorMessage(message string) *Error {
	lower := strings.ToLower(message)

	switch {
	case strings.Contains(lower, "context deadline exceeded"):
		return New(CodeQueryTimeout, message)
	case strings.Contains(lower, "error 1054") || strings.Contains(lower, "unknown column"):
		return New(CodeSQLUnknownColumn, message)
	case strings.Contains(lower, "error 1146") || (strings.Contains(lower, "table") && strings.Contains(lower, "doesn't exist")):
		return New(CodeSQLUnknownTable, message)
	case strings.Contains(lower, "error 1064") || strings.Contains(lower, "sql syntax"):
		return New(CodeQuerySyntaxError, message)
	case strings.Contains(lower, "broken pipe") || strings.Contains(lower, "connection reset by peer"):
		return New(CodeConnectionFailed, message)
	default:
		return New(CodeDriverError, message)
	}
}
