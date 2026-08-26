package verification

import (
	"isolation-chamber-commissioning/internal/domain"
	"testing"
	"time"
)

func limits() domain.AcceptanceLimits {
	return domain.AcceptanceLimits{PressureMinPa: 15, PressureDurationSec: 60, MaxLeakagePercent: 5, InterlockResponseSec: 2, RecoveryMaxMinutes: 20, RecoveryTargetParticles: 3520}
}
func input(kind domain.CheckpointKind, id string, m []domain.Measurement) Input {
	now := time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)
	return Input{CaseID: "case-1", ProtocolRevision: 1, Checkpoint: domain.Checkpoint{ID: id, Kind: kind}, Limits: limits(), Measurements: m, InstrumentID: "INST-1", Witness: "见证人", StartedAt: now, CompletedAt: now.Add(time.Minute), Attempt: 1, RunID: "run-1"}
}

func TestPressureUsesMinimumAndSamplingWindow(t *testing.T) {
	run, err := New().Evaluate(input(domain.KindPressure, "pressure", []domain.Measurement{{Name: "pressurePa", Value: 18, Unit: "Pa", OffsetSec: 0}, {Name: "pressurePa", Value: 14, Unit: "Pa", OffsetSec: 30}}))
	if err != nil {
		t.Fatal(err)
	}
	if run.Verdict != domain.VerdictFail || len(run.FailureReasons) != 2 {
		t.Fatalf("判定=%+v", run)
	}
	digest, err := EvidenceDigest(run)
	if err != nil || digest != run.EvidenceDigest {
		t.Fatalf("证据摘要不可复算 %s %v", digest, err)
	}
}
func TestAllKindsPassing(t *testing.T) {
	no := false
	tests := []Input{input(domain.KindPressure, "pressure", []domain.Measurement{{Name: "pressurePa", Value: 18, OffsetSec: 0}, {Name: "pressurePa", Value: 17, OffsetSec: 60}}), input(domain.KindAirtightness, "airtightness", []domain.Measurement{{Name: "leakagePercent", Value: 3}}), input(domain.KindInterlock, "interlock", []domain.Measurement{{Name: "responseSec", Value: 1.2}, {Name: "bothDoorsOpen", Flag: &no}}), input(domain.KindRecovery, "recovery", []domain.Measurement{{Name: "recoveryMinutes", Value: 12}, {Name: "particleCount", Value: 2800}})}
	for _, in := range tests {
		run, err := New().Evaluate(in)
		if err != nil || run.Verdict != domain.VerdictPass {
			t.Fatalf("%s: %+v %v", in.Checkpoint.ID, run, err)
		}
	}
}
func TestCredentialDigestDetectsChange(t *testing.T) {
	snapshot := domain.Snapshot{CaseID: "case-1", CaseNumber: "ICC-1", ChamberName: "隔离舱"}
	sd, err := SnapshotDigest(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	cd, err := CredentialDigest("AC-1", "case-1", "ICC-1", "复核员", now, sd, 1)
	if err != nil {
		t.Fatal(err)
	}
	cred := domain.ActivationCredential{ID: "AC-1", CaseID: "case-1", CaseNumber: "ICC-1", ApprovedBy: "复核员", ApprovedAt: now, SnapshotDigest: sd, CredentialDigest: cd, SchemaVersion: 1}
	ok, _, err := VerifyCredential(cred, snapshot)
	if err != nil || !ok {
		t.Fatal("原始凭据应真实")
	}
	snapshot.ChamberName = "被修改"
	ok, _, err = VerifyCredential(cred, snapshot)
	if err != nil || ok {
		t.Fatal("修改封存快照应被识别")
	}
}
