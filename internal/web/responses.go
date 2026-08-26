package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"isolation-chamber-commissioning/internal/application"
	"isolation-chamber-commissioning/internal/domain"
)

func caseParts(r *http.Request) []string {
	return strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/cases/"), "/"), "/")
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	if ct := r.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, "application/json") {
		writeError(w, 415, "UNSUPPORTED_MEDIA_TYPE", "请求必须使用 application/json", "")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, 400, "BAD_JSON", "JSON 请求无效："+err.Error(), "")
		return false
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		writeError(w, 400, "BAD_JSON", "请求只能包含一个 JSON 对象", "")
		return false
	}
	return true
}

func respondWrite(w http.ResponseWriter, result application.WriteResult, err error) {
	if err != nil {
		respondError(w, err)
		return
	}
	if result.Replayed {
		w.Header().Set("Idempotency-Replayed", "true")
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(result.Status)
	_, _ = w.Write(result.Body)
}
func respondError(w http.ResponseWriter, err error) {
	var de *domain.DomainError
	if errors.As(err, &de) {
		status := 400
		switch de.Code {
		case domain.CodeConflict:
			status = 409
		case domain.CodeNotFound:
			status = 404
		case domain.CodeDuplicate:
			status = 409
		case domain.CodeState:
			status = 422
		}
		payload := map[string]any{"code": string(de.Code), "message": de.Message}
		if de.Field != "" {
			payload["field"] = de.Field
		}
		if de.CurrentVersion != 0 {
			payload["currentVersion"] = de.CurrentVersion
		}
		if len(de.ConflictFields) > 0 {
			payload["conflictFields"] = de.ConflictFields
		}
		writeErrorPayload(w, status, payload)
		return
	}
	writeError(w, 500, "INTERNAL", "服务内部错误", "")
}
func writeError(w http.ResponseWriter, status int, code, message, field string) {
	payload := map[string]any{"code": code, "message": message}
	if field != "" {
		payload["field"] = field
	}
	writeErrorPayload(w, status, payload)
}
func writeErrorPayload(w http.ResponseWriter, status int, payload map[string]any) {
	writeJSON(w, status, map[string]any{"error": payload})
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		fmt.Printf("encode response: %v\n", err)
	}
}
func methodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	writeError(w, 405, "METHOD_NOT_ALLOWED", "请求方法不受支持", "")
}
