package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"pdconsole/internal/application"
	"pdconsole/internal/domain"
	"pdconsole/internal/persistence"
)

type errorResponse struct {
	Error   string              `json:"error"`
	Code    string              `json:"code"`
	Fields  []domain.FieldError `json:"fields,omitempty"`
	Current any                 `json:"current,omitempty"`
}

func decodeJSON(writer http.ResponseWriter, request *http.Request, target any) bool {
	contentType := request.Header.Get("Content-Type")
	if !strings.HasPrefix(strings.ToLower(contentType), "application/json") {
		writeJSON(writer, http.StatusUnsupportedMediaType, errorResponse{Error: "请求必须使用 application/json", Code: "unsupported_media_type"})
		return false
	}
	request.Body = http.MaxBytesReader(writer, request.Body, 1<<20)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeJSON(writer, http.StatusBadRequest, errorResponse{Error: friendlyDecodeError(err), Code: "invalid_json"})
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeJSON(writer, http.StatusBadRequest, errorResponse{Error: "请求体只能包含一个 JSON 对象", Code: "invalid_json"})
		return false
	}
	return true
}

func friendlyDecodeError(err error) string {
	var syntax *json.SyntaxError
	var typeError *json.UnmarshalTypeError
	switch {
	case errors.As(err, &syntax):
		return fmt.Sprintf("JSON 在第 %d 字节附近格式错误", syntax.Offset)
	case errors.As(err, &typeError):
		return fmt.Sprintf("字段 %s 的数据类型不正确", typeError.Field)
	case errors.Is(err, io.EOF):
		return "请求体不能为空"
	case strings.HasPrefix(err.Error(), "json: unknown field"):
		return "请求包含未知字段: " + strings.TrimPrefix(err.Error(), "json: unknown field ")
	case strings.Contains(err.Error(), "http: request body too large"):
		return "请求体超过 1 MiB 限制"
	default:
		return "无法解析 JSON 请求: " + err.Error()
	}
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeError(writer http.ResponseWriter, err error) {
	var fieldError domain.FieldError
	var fieldErrors domain.ValidationErrors
	var conflict application.VersionConflictError
	var duplicate application.DuplicateBatchError
	switch {
	case errors.As(err, &duplicate):
		writeJSON(writer, http.StatusConflict, errorResponse{Error: duplicate.Error(), Code: "duplicate_batch", Current: map[string]any{"matches": duplicate.Matches}})
	case errors.As(err, &conflict):
		writeJSON(writer, http.StatusConflict, errorResponse{
			Error: conflict.Error(), Code: "version_conflict",
			Current: map[string]any{"batchID": conflict.BatchID, "version": conflict.Actual},
		})
	case errors.As(err, &fieldErrors):
		writeJSON(writer, http.StatusUnprocessableEntity, errorResponse{Error: "整批字段校验失败", Code: "validation_error", Fields: []domain.FieldError(fieldErrors)})
	case errors.As(err, &fieldError):
		writeJSON(writer, http.StatusUnprocessableEntity, errorResponse{Error: "字段校验失败", Code: "validation_error", Fields: []domain.FieldError{fieldError}})
	case errors.Is(err, persistence.ErrNotFound), errors.Is(err, domain.ErrNotFound):
		writeJSON(writer, http.StatusNotFound, errorResponse{Error: err.Error(), Code: "not_found"})
	case errors.Is(err, application.ErrIdempotencyKey):
		writeJSON(writer, http.StatusBadRequest, errorResponse{Error: "必须提供 idempotencyKey", Code: "missing_idempotency_key"})
	case errors.Is(err, domain.ErrBatchSealed), errors.Is(err, domain.ErrReviewLocked):
		writeJSON(writer, http.StatusConflict, errorResponse{Error: err.Error(), Code: "resource_locked"})
	case errors.Is(err, domain.ErrInvalidTransition), errors.Is(err, domain.ErrScopeFrozen), errors.Is(err, domain.ErrOpenDeviation):
		writeJSON(writer, http.StatusConflict, errorResponse{Error: err.Error(), Code: "invalid_transition"})
	default:
		writeJSON(writer, http.StatusInternalServerError, errorResponse{Error: "服务处理失败: " + err.Error(), Code: "internal_error"})
	}
}
