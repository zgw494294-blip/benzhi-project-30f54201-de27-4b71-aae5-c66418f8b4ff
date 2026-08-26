package web

import (
	"net/http"
	"strings"
)

func (h *APIHandler) routeCase(w http.ResponseWriter, r *http.Request) {
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/cases/"), "/")
	parts := strings.Split(rest, "/")
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, 404, "NOT_FOUND", "验证案路径无效", "")
		return
	}
	if len(parts) == 1 {
		if r.Method == http.MethodGet {
			h.GetCaseHandler(w, r)
		} else if r.Method == http.MethodPut || r.Method == http.MethodPatch {
			h.ReviseCaseHandler(w, r)
		} else {
			methodNotAllowed(w, "GET, PUT, PATCH")
		}
		return
	}
	if len(parts) == 2 && (parts[1] == "revise" || parts[1] == "revision" || parts[1] == "revisions") {
		if r.Method != http.MethodPost {
			methodNotAllowed(w, "POST")
			return
		}
		h.ReviseCaseHandler(w, r)
		return
	}
	if (len(parts) == 2 && (parts[1] == "preflight" || parts[1] == "freeze-preflight")) || (len(parts) == 3 && parts[1] == "freeze" && parts[2] == "preflight") {
		if r.Method != http.MethodPost {
			methodNotAllowed(w, "POST")
			return
		}
		h.FreezePreflightHandler(w, r)
		return
	}
	if len(parts) == 2 && parts[1] == "freeze" {
		if r.Method != http.MethodPost {
			methodNotAllowed(w, "POST")
			return
		}
		h.FreezeProtocolHandler(w, r)
		return
	}
	if len(parts) == 2 && parts[1] == "runs" {
		if r.Method != http.MethodPost {
			methodNotAllowed(w, "POST")
			return
		}
		h.RecordRunHandler(w, r)
		return
	}
	if len(parts) == 2 && parts[1] == "submit-review" {
		if r.Method != http.MethodPost {
			methodNotAllowed(w, "POST")
			return
		}
		h.SubmitReviewHandler(w, r)
		return
	}
	if len(parts) == 2 && parts[1] == "review" {
		if r.Method != http.MethodPost {
			methodNotAllowed(w, "POST")
			return
		}
		h.ReviewHandler(w, r)
		return
	}
	if len(parts) == 4 && parts[1] == "deviations" && parts[3] == "remediate" {
		if r.Method != http.MethodPost {
			methodNotAllowed(w, "POST")
			return
		}
		h.RemediateHandler(w, r)
		return
	}
	writeError(w, 404, "NOT_FOUND", "案卷操作不存在", "")
}
