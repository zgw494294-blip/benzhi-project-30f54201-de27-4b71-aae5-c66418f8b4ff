package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"isolation-chamber-commissioning/internal/application"
)

//go:embed static/*
var assets embed.FS

type APIHandler struct {
	service *application.Service
	static  http.Handler
}

func NewHandler(service *application.Service) http.Handler {
	sub, _ := fs.Sub(assets, "static")
	return &APIHandler{service: service, static: http.FileServer(http.FS(sub))}
}

func (h *APIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "same-origin")
	if r.URL.Path == "/healthz" {
		h.HealthHandler(w, r)
		return
	}
	if r.URL.Path == "/api/system" {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, "GET")
			return
		}
		h.SystemStatusHandler(w, r)
		return
	}
	if r.URL.Path == "/" {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, "GET")
			return
		}
		h.IndexHandler(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/assets/") {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, "GET")
			return
		}
		r2 := r.Clone(r.Context())
		r2.URL.Path = strings.TrimPrefix(r.URL.Path, "/assets")
		h.static.ServeHTTP(w, r2)
		return
	}
	if r.URL.Path == "/api/cases" {
		switch r.Method {
		case http.MethodGet:
			h.ListCasesHandler(w, r)
		case http.MethodPost:
			h.CreateCaseHandler(w, r)
		default:
			methodNotAllowed(w, "GET, POST")
		}
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/credentials/") && strings.HasSuffix(r.URL.Path, "/verify") {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, "GET")
			return
		}
		h.VerifyCredentialHandler(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/cases/") {
		h.routeCase(w, r)
		return
	}
	writeError(w, http.StatusNotFound, "NOT_FOUND", "路由不存在", "")
}
