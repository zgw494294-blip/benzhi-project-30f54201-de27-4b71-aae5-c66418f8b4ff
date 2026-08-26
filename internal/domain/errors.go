package domain

import "fmt"

type ErrorCode string

const (
	CodeValidation ErrorCode = "VALIDATION"
	CodeConflict   ErrorCode = "VERSION_CONFLICT"
	CodeState      ErrorCode = "INVALID_STATE"
	CodeNotFound   ErrorCode = "NOT_FOUND"
	CodeDuplicate  ErrorCode = "DUPLICATE"
)

type DomainError struct {
	Code           ErrorCode `json:"code"`
	Message        string    `json:"message"`
	Field          string    `json:"field,omitempty"`
	CurrentVersion int       `json:"currentVersion,omitempty"`
	ConflictFields []string  `json:"conflictFields,omitempty"`
}

func (e *DomainError) Error() string { return e.Message }

func Validation(field, message string) error {
	return &DomainError{Code: CodeValidation, Field: field, Message: message}
}

func State(message string) error { return &DomainError{Code: CodeState, Message: message} }

func Conflict(expected, actual int) error {
	return &DomainError{Code: CodeConflict, Message: fmt.Sprintf("版本冲突：期望 %d，当前 %d", expected, actual), CurrentVersion: actual}
}

func RevisionConflict(expected, actual int, fields []string) error {
	return &DomainError{Code: CodeConflict, Message: fmt.Sprintf("案卷已被修订：期望版本 %d，当前版本 %d", expected, actual), CurrentVersion: actual, ConflictFields: fields}
}

func NotFound(kind, id string) error {
	return &DomainError{Code: CodeNotFound, Message: fmt.Sprintf("%s不存在：%s", kind, id)}
}

func Duplicate(message string) error { return &DomainError{Code: CodeDuplicate, Message: message} }
