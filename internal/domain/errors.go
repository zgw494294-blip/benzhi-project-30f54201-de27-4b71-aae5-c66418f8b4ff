package domain

import "errors"

var (
	ErrInvalidInput      = errors.New("输入数据不合法")
	ErrInvalidTransition = errors.New("当前状态不允许执行该操作")
	ErrScopeFrozen       = errors.New("试验边界已经冻结")
	ErrBatchSealed       = errors.New("批次已封存，不允许变更")
	ErrNotFound          = errors.New("业务对象不存在")
	ErrReviewLocked      = errors.New("复核结论已经锁定")
	ErrOpenDeviation     = errors.New("仍有未关闭的诊断偏差")
)

type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e FieldError) Error() string { return e.Field + ": " + e.Message }

// ValidationErrors 用于一次返回整批表格中的全部字段错误，避免操作员逐次修正。
type ValidationErrors []FieldError

func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return ErrInvalidInput.Error()
	}
	return e[0].Error()
}

func (e ValidationErrors) Unwrap() error { return ErrInvalidInput }

func invalid(field, message string) error {
	return FieldError{Field: field, Message: message}
}
