package persistence

import (
	"encoding/json"
	"isolation-chamber-commissioning/internal/domain"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func storeCase(t *testing.T, id string) *domain.CommissioningCase {
	t.Helper()
	c, err := domain.CreateCase(domain.NewCase{ID: id, CaseNumber: "ICC-1", ChamberName: "隔离舱", Zones: []domain.ZoneBoundary{{Chamber: "隔离舱", Adjacent: "走廊"}}, AirflowDirection: "走廊 → 隔离舱", Limits: domain.AcceptanceLimits{PressureMinPa: 15, PressureDurationSec: 60, MaxLeakagePercent: 5, InterlockResponseSec: 2, RecoveryMaxMinutes: 20, RecoveryTargetParticles: 3520}, Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func TestCommitIdempotencyVersionAndRecovery(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	c := storeCase(t, "case-1")
	body, _ := json.Marshal(c)
	commit := Commit{IdempotencyKey: "create:k", ExpectedVersion: 0, Case: c, Action: "CASE_CREATED", Result: CommandResult{Status: 201, Body: body}, Now: time.Now()}
	first, replayed, err := s.Commit(commit)
	if err != nil || replayed || first.Status != 201 {
		t.Fatalf("首次提交 %+v %v %v", first, replayed, err)
	}
	_, replayed, err = s.Commit(commit)
	if err != nil || !replayed {
		t.Fatal("相同幂等键应重放")
	}
	other := storeCase(t, "case-1")
	other.Version = 2
	_, _, err = s.Commit(Commit{IdempotencyKey: "update:k", ExpectedVersion: 0, Case: other, Action: "UPDATE", Result: CommandResult{Status: 200}, Now: time.Now()})
	if err == nil {
		t.Fatal("应拒绝陈旧存量版本")
	}
	s.Close()
	recovered, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	got, err := recovered.GetCase("case-1")
	if err != nil || got.Version != 1 {
		t.Fatalf("恢复案卷 %+v %v", got, err)
	}
	events, err := recovered.AuditEvents("case-1")
	if err != nil || len(events) != 1 {
		t.Fatalf("恢复审计链 %+v %v", events, err)
	}
}
func TestOpenRejectsTamperedAuditChain(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	c := storeCase(t, "case-2")
	body, _ := json.Marshal(c)
	_, _, err = s.Commit(Commit{IdempotencyKey: "create:k", ExpectedVersion: 0, Case: c, Action: "CREATE", Result: CommandResult{Status: 201, Body: body}, Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	path := filepath.Join(dir, "audit.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data[len(data)/2] ^= 1
	if err := os.WriteFile(path, data, 0640); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir); err == nil {
		t.Fatal("篡改审计日志后启动必须失败")
	}
}
