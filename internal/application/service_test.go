package application

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"isolation-chamber-commissioning/internal/domain"
	"isolation-chamber-commissioning/internal/persistence"
	"isolation-chamber-commissioning/internal/verification"
)

func workflowService(t *testing.T) (*Service, *persistence.Store, time.Time) {
	t.Helper()
	store, err := persistence.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	now := time.Date(2026, 8, 26, 9, 0, 0, 0, time.UTC)
	return NewWithClock(store, verification.New(), func() time.Time { return now }), store, now
}

func createWorkflowCase(t *testing.T, service *Service) *domain.CommissioningCase {
	t.Helper()
	limits := domain.AcceptanceLimits{PressureMinPa: 15, PressureDurationSec: 60, MaxLeakagePercent: 5, InterlockResponseSec: 2, RecoveryMaxMinutes: 20, RecoveryTargetParticles: 3520}
	result, err := service.CreateCase(CreateCaseCommand{ChamberName: "隔离舱 A", Zones: []domain.ZoneBoundary{{Chamber: "隔离舱 A", Adjacent: "洁净走廊"}}, AirflowDirection: "洁净走廊 → 隔离舱 A", AcceptanceLimits: limits, IdempotencyKey: "create"})
	if err != nil {
		t.Fatal(err)
	}
	var c domain.CommissioningCase
	if err := json.Unmarshal(result.Body, &c); err != nil {
		t.Fatal(err)
	}
	return &c
}

func TestRevisionPreflightAndStaleConfirmation(t *testing.T) {
	service, _, _ := workflowService(t)
	c := createWorkflowCase(t, service)
	limits := c.AcceptanceLimits
	limits.PressureMinPa = 18
	revised, err := service.ReviseCase(c.ID, ReviseCaseCommand{ExpectedVersion: c.Version, ChamberName: "  隔离舱   A ", Zones: []domain.ZoneBoundary{{Chamber: "隔离舱 A", Adjacent: "洁净走廊"}, {Chamber: "隔离舱 A", Adjacent: "设备间"}}, AirflowDirection: "洁净走廊->隔离舱 A", AcceptanceLimits: limits, Actor: "工程师", IdempotencyKey: "revise"})
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(revised.Body, c); err != nil {
		t.Fatal(err)
	}
	if c.Version != 2 || len(c.Zones) != 2 || c.ChamberName != "隔离舱 A" {
		t.Fatalf("修订结果异常：%+v", c)
	}
	preflight, err := service.FreezePreflight(c.ID, FreezePreflightCommand{ExpectedVersion: c.Version, Actor: "工程师", IdempotencyKey: "preflight"})
	if err != nil {
		t.Fatal(err)
	}
	var preview struct {
		Report domain.FreezeConfirmation `json:"report"`
	}
	if err := json.Unmarshal(preflight.Body, &preview); err != nil {
		t.Fatal(err)
	}
	limits.PressureMinPa = 19
	if _, err := service.ReviseCase(c.ID, ReviseCaseCommand{ExpectedVersion: c.Version, ChamberName: c.ChamberName, Zones: c.Zones, AirflowDirection: c.AirflowDirection, AcceptanceLimits: limits, IdempotencyKey: "revise-again"}); err != nil {
		t.Fatal(err)
	}
	_, err = service.FreezeProtocol(c.ID, FreezeProtocolCommand{ExpectedVersion: c.Version, FrozenBy: "工程师", ConfirmationToken: preview.Report.Token, IdempotencyKey: "stale-freeze"})
	var de *domain.DomainError
	if !errors.As(err, &de) || de.Code != domain.CodeConflict || de.CurrentVersion != 3 {
		t.Fatalf("旧确认标识错误=%+v", err)
	}
}

func TestBatchValidationIsAtomicAndSuccessCommitsOnce(t *testing.T) {
	service, store, now := workflowService(t)
	c := createWorkflowCase(t, service)
	p, err := service.FreezePreflight(c.ID, FreezePreflightCommand{ExpectedVersion: c.Version, IdempotencyKey: "preflight"})
	if err != nil {
		t.Fatal(err)
	}
	var preview struct {
		Report domain.FreezeConfirmation `json:"report"`
	}
	if err := json.Unmarshal(p.Body, &preview); err != nil {
		t.Fatal(err)
	}
	frozen, err := service.FreezeProtocol(c.ID, FreezeProtocolCommand{ExpectedVersion: c.Version, ConfirmationToken: preview.Report.Token, IdempotencyKey: "freeze"})
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(frozen.Body, c); err != nil {
		t.Fatal(err)
	}
	falseValue := false
	item := func(id, witness string, measurements []domain.Measurement, kind domain.CheckpointKind) RunItemCommand {
		return RunItemCommand{CheckpointID: id, Measurements: measurements, InstrumentID: "INST-1", CertificateNumber: "CAL-1", CalibrationValidFrom: now.Add(-time.Hour), CalibrationValidUntil: now.Add(time.Hour), ApplicableKinds: []domain.CheckpointKind{kind}, Witness: witness, CompletedAt: now}
	}
	pressure := []domain.Measurement{{Name: "pressurePa", Value: 18, Unit: "Pa"}, {Name: "pressurePa", Value: 17, Unit: "Pa", OffsetSec: 60}}
	airtight := []domain.Measurement{{Name: "leakagePercent", Value: 3, Unit: "%"}}
	interlock := []domain.Measurement{{Name: "responseSec", Value: 1, Unit: "s"}, {Name: "bothDoorsOpen", Unit: "boolean", Flag: &falseValue}}
	bad := RecordRunCommand{ExpectedVersion: c.Version, ProtocolRevision: c.Protocol.Revision, Runs: []RunItemCommand{item("pressure", "见证人", pressure, domain.KindPressure), item("airtightness", "", airtight, domain.KindAirtightness)}, IdempotencyKey: "bad-batch"}
	if _, err := service.RecordRun(c.ID, bad); err == nil {
		t.Fatal("缺少见证人的批次应失败")
	}
	unchanged, err := store.GetCase(c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(unchanged.Runs) != 0 || unchanged.Version != c.Version {
		t.Fatalf("失败批次发生写入：%+v", unchanged.Runs)
	}
	good := RecordRunCommand{ExpectedVersion: c.Version, ProtocolRevision: c.Protocol.Revision, Runs: []RunItemCommand{item("pressure", "见证人", pressure, domain.KindPressure), item("airtightness", "见证人", airtight, domain.KindAirtightness), item("interlock", "见证人", interlock, domain.KindInterlock)}, IdempotencyKey: "good-batch"}
	result, err := service.RecordRun(c.ID, good)
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		Case *domain.CommissioningCase `json:"case"`
		Runs []domain.TestRun          `json:"runs"`
	}
	if err := json.Unmarshal(result.Body, &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Runs) != 3 || response.Case.Version != c.Version+1 {
		t.Fatalf("批量结果异常：%+v", response)
	}
	replay, err := service.RecordRun(c.ID, good)
	if err != nil || !replay.Replayed {
		t.Fatalf("批次幂等重放失败：%v %+v", err, replay)
	}
}
