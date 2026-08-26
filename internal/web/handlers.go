package web

import (
	"net/http"
	"strings"

	"isolation-chamber-commissioning/internal/application"
)

func (h *APIHandler) HealthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, "GET")
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}
func (h *APIHandler) SystemStatusHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, h.service.Status())
}
func (h *APIHandler) IndexHandler(w http.ResponseWriter, r *http.Request) {
	b, err := assets.ReadFile("static/index.html")
	if err != nil {
		writeError(w, 500, "INTERNAL", err.Error(), "")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(200)
	_, _ = w.Write(b)
}
func (h *APIHandler) ListCasesHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"cases": h.service.ListCases(r.URL.Query().Get("q"))})
}

func (h *APIHandler) CreateCaseHandler(w http.ResponseWriter, r *http.Request) {
	var cmd application.CreateCaseCommand
	if !decode(w, r, &cmd) {
		return
	}
	cmd.IdempotencyKey = r.Header.Get("Idempotency-Key")
	result, err := h.service.CreateCase(cmd)
	respondWrite(w, result, err)
}
func (h *APIHandler) GetCaseHandler(w http.ResponseWriter, r *http.Request) {
	id := caseParts(r)[0]
	value, err := h.service.GetCase(id)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, 200, value)
}
func (h *APIHandler) ReviseCaseHandler(w http.ResponseWriter, r *http.Request) {
	var cmd application.ReviseCaseCommand
	if !decode(w, r, &cmd) {
		return
	}
	cmd.IdempotencyKey = r.Header.Get("Idempotency-Key")
	result, err := h.service.ReviseCase(caseParts(r)[0], cmd)
	respondWrite(w, result, err)
}
func (h *APIHandler) FreezePreflightHandler(w http.ResponseWriter, r *http.Request) {
	var cmd application.FreezePreflightCommand
	if !decode(w, r, &cmd) {
		return
	}
	cmd.IdempotencyKey = r.Header.Get("Idempotency-Key")
	result, err := h.service.FreezePreflight(caseParts(r)[0], cmd)
	respondWrite(w, result, err)
}
func (h *APIHandler) FreezeProtocolHandler(w http.ResponseWriter, r *http.Request) {
	var cmd application.FreezeProtocolCommand
	if !decode(w, r, &cmd) {
		return
	}
	cmd.IdempotencyKey = r.Header.Get("Idempotency-Key")
	result, err := h.service.FreezeProtocol(caseParts(r)[0], cmd)
	respondWrite(w, result, err)
}
func (h *APIHandler) RecordRunHandler(w http.ResponseWriter, r *http.Request) {
	var cmd application.RecordRunCommand
	if !decode(w, r, &cmd) {
		return
	}
	cmd.IdempotencyKey = r.Header.Get("Idempotency-Key")
	result, err := h.service.RecordRun(caseParts(r)[0], cmd)
	respondWrite(w, result, err)
}
func (h *APIHandler) RemediateHandler(w http.ResponseWriter, r *http.Request) {
	var cmd application.RemediateCommand
	if !decode(w, r, &cmd) {
		return
	}
	cmd.IdempotencyKey = r.Header.Get("Idempotency-Key")
	parts := caseParts(r)
	result, err := h.service.Remediate(parts[0], parts[2], cmd)
	respondWrite(w, result, err)
}
func (h *APIHandler) SubmitReviewHandler(w http.ResponseWriter, r *http.Request) {
	var cmd application.SubmitReviewCommand
	if !decode(w, r, &cmd) {
		return
	}
	cmd.IdempotencyKey = r.Header.Get("Idempotency-Key")
	result, err := h.service.SubmitReview(caseParts(r)[0], cmd)
	respondWrite(w, result, err)
}
func (h *APIHandler) ReviewHandler(w http.ResponseWriter, r *http.Request) {
	var cmd application.ReviewCommand
	if !decode(w, r, &cmd) {
		return
	}
	cmd.IdempotencyKey = r.Header.Get("Idempotency-Key")
	result, err := h.service.Review(caseParts(r)[0], cmd)
	respondWrite(w, result, err)
}
func (h *APIHandler) VerifyCredentialHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/credentials/"), "/verify")
	id = strings.Trim(id, "/")
	value, err := h.service.VerifyCredential(id)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, 200, value)
}
