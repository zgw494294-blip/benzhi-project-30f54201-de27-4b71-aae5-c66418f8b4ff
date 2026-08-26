package web

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"pdconsole/internal/application"
	"pdconsole/internal/persistence"
)

func testServer(t *testing.T) http.Handler {
	t.Helper()
	store, err := persistence.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return NewServer(application.NewService(store)).Handler()
}

func TestIndexAndHealth(t *testing.T) {
	handler := testServer(t)
	for _, path := range []string{"/", "/api/health"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s 返回 %d", path, response.Code)
		}
	}
}

func TestCreateValidationAndContentType(t *testing.T) {
	handler := testServer(t)
	request := httptest.NewRequest(http.MethodPost, "/api/batches", bytes.NewBufferString(`{"idempotencyKey":"k"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("缺少字段应返回 422，得到 %d: %s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodPost, "/api/batches", bytes.NewBufferString(`{}`))
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("错误 Content-Type 应返回 415，得到 %d", response.Code)
	}
}
