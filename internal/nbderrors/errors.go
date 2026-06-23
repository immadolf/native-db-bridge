package nbderrors

// Category groups error codes into high-level buckets.
type Category string

// Code uniquely identifies an error condition.
type Code string

const (
	CategoryConfig     Category = "config"
	CategoryPolicy     Category = "policy"
	CategoryConnection Category = "connection"
	CategorySyntax     Category = "syntax"
	CategoryTimeout    Category = "timeout"
	CategoryDriver     Category = "driver"
	CategoryInternal   Category = "internal"
)

const (
	CodeConfigFileNotFound              Code = "CONFIG_FILE_NOT_FOUND"
	CodeConfigPermissionTooOpen         Code = "CONFIG_PERMISSION_TOO_OPEN"
	CodeConfigDatasourceNotFound        Code = "CONFIG_DATASOURCE_NOT_FOUND"
	CodeConfigProductionRejected        Code = "CONFIG_PRODUCTION_DATASOURCE_REJECTED"
	CodeAuthMissingToken                Code = "AUTH_MISSING_TOKEN"
	CodeAuthInvalidToken                Code = "AUTH_INVALID_TOKEN"
	CodePolicyWriteRequiresConfirm      Code = "POLICY_WRITE_REQUIRES_CONFIRMATION"
	CodePolicyProductionRejected        Code = "POLICY_PRODUCTION_REJECTED"
	CodePolicyRedisSelectRejected       Code = "POLICY_REDIS_SELECT_REJECTED"
	CodePolicyReadonlyToolRejectedWrite Code = "POLICY_READONLY_TOOL_REJECTED_WRITE"
	CodeConnectionFailed                Code = "CONNECTION_FAILED"
	CodeConnectionAuthFailed            Code = "CONNECTION_AUTH_FAILED"
	CodeQueryTimeout                    Code = "QUERY_TIMEOUT"
	CodeQuerySyntaxError                Code = "QUERY_SYNTAX_ERROR"
	CodeSQLUnknownColumn                Code = "SQL_UNKNOWN_COLUMN"
	CodeSQLUnknownTable                 Code = "SQL_UNKNOWN_TABLE"
	CodeQueryLockingReadRejected        Code = "QUERY_LOCKING_READ_REJECTED"
	CodeConfirmationNotFound            Code = "CONFIRMATION_NOT_FOUND"
	CodeConfirmationExpired             Code = "CONFIRMATION_EXPIRED"
	CodeConfirmationAlreadyExecuted     Code = "CONFIRMATION_ALREADY_EXECUTED"
	CodeConfirmationInvalidState        Code = "CONFIRMATION_INVALID_STATE"
	CodeOperationNotFound               Code = "OPERATION_NOT_FOUND"
	CodeOperationNotCancelable          Code = "OPERATION_NOT_CANCELABLE"
	CodeDriverError                     Code = "DRIVER_ERROR"
	CodeInternalError                   Code = "INTERNAL_ERROR"
)

// Error is a structured error with code, category, and machine-readable metadata.
type Error struct {
	Code        Code                   `json:"code"`
	Category    Category               `json:"category"`
	Message     string                 `json:"message"`
	Datasource  string                 `json:"datasource,omitempty"`
	OperationID string                 `json:"operation_id,omitempty"`
	Retryable   bool                   `json:"retryable"`
	Details     map[string]interface{} `json:"details,omitempty"`
}

// Error implements the Go error interface.
func (e *Error) Error() string {
	return string(e.Code) + ": " + e.Message
}

// New creates an Error with auto-assigned category and retryable flag.
func New(code Code, message string) *Error {
	return &Error{
		Code:      code,
		Category:  categoryFor(code),
		Message:   message,
		Retryable: retryableFor(code),
	}
}

// WithDatasource sets the datasource field and returns the error for chaining.
func (e *Error) WithDatasource(ds string) *Error {
	e.Datasource = ds
	return e
}

// WithOperationID sets the operation ID and returns the error for chaining.
func (e *Error) WithOperationID(id string) *Error {
	e.OperationID = id
	return e
}

// WithDetails merges extra key-value pairs into the error details.
func (e *Error) WithDetails(key string, value interface{}) *Error {
	if e.Details == nil {
		e.Details = make(map[string]interface{})
	}
	e.Details[key] = value
	return e
}

func categoryFor(code Code) Category {
	switch code {
	case CodeConfigFileNotFound, CodeConfigPermissionTooOpen, CodeConfigDatasourceNotFound, CodeConfigProductionRejected:
		return CategoryConfig
	case CodeAuthMissingToken, CodeAuthInvalidToken:
		return CategoryPolicy
	case CodeConnectionFailed, CodeConnectionAuthFailed:
		return CategoryConnection
	case CodeQuerySyntaxError, CodeSQLUnknownColumn, CodeSQLUnknownTable:
		return CategorySyntax
	case CodeQueryTimeout:
		return CategoryTimeout
	case CodeDriverError:
		return CategoryDriver
	case CodeInternalError:
		return CategoryInternal
	default:
		return CategoryPolicy
	}
}

func retryableFor(code Code) bool {
	return code == CodeConnectionFailed || code == CodeQueryTimeout || code == CodeDriverError
}
