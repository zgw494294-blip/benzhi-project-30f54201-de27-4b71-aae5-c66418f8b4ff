package web

import (
	"bytes"
	"encoding/json"
	"isolation-chamber-commissioning/internal/application"
	"isolation-chamber-commissioning/internal/domain"
	"isolation-chamber-commissioning/internal/persistence"
	"isolation-chamber-commissioning/internal/verification"
	"net/http"
	"net/http/httptest"
	"testing"
)

func testHandler(t *testing.T) http.Handler {
	t.Helper()
	store, err := persistence.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return NewHandler(application.New(store, verification.New()))
}
func TestIndexAndCreateAPI(t *testing.T) {
	h := testHandler(t)
	index := httptest.NewRecorder()
	h.ServeHTTP(index, httptest.NewRequest(http.MethodGet, "/", nil))
	if index.Code != 200 || !bytes.Contains(index.Body.Bytes(), []byte("<body>")) {
		t.Fatalf("首页响应 %d", index.Code)
	}
	payload := application.CreateCaseCommand{ChamberName: "Web 隔离舱", Zones: []domain.ZoneBoundary{{Chamber: "Web 隔离舱", Adjacent: "走廊"}}, AirflowDirection: "走廊 → 隔离舱", AcceptanceLimits: domain.AcceptanceLimits{PressureMinPa: 15, PressureDurationSec: 60, MaxLeakagePercent: 5, InterlockResponseSec: 2, RecoveryMaxMinutes: 20, RecoveryTargetParticles: 3520}}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/cases", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "web-create")
	response := httptest.NewRecorder()
	h.ServeHTTP(response, req)
	if response.Code != 201 {
		t.Fatalf("建案响应 %d %s", response.Code, response.Body.String())
	}
	replay := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/cases", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "web-create")
	h.ServeHTTP(replay, req)
	if replay.Header().Get("Idempotency-Replayed") != "true" {
		t.Fatal("HTTP 未标记幂等重放")
	}
}
func TestAPIRejectsUnknownFieldAndMissingKey(t *testing.T) {
	h := testHandler(t)
	bad := httptest.NewRequest(http.MethodPost, "/api/cases", bytes.NewBufferString(`{"unknown":true}`))
	bad.Header.Set("Content-Type", "application/json")
	bad.Header.Set("Idempotency-Key", "bad")
	r := httptest.NewRecorder()
	h.ServeHTTP(r, bad)
	if r.Code != 400 {
		t.Fatalf("未知字段响应 %d", r.Code)
	}
	empty := httptest.NewRequest(http.MethodPost, "/api/cases", bytes.NewBufferString(`{}`))
	empty.Header.Set("Content-Type", "application/json")
	r = httptest.NewRecorder()
	h.ServeHTTP(r, empty)
	if r.Code != 400 {
		t.Fatalf("缺幂等键响应 %d", r.Code)
	}
}
