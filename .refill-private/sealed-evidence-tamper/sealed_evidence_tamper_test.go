package sealed_evidence_tamper_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"pdconsole/internal/application"
	"pdconsole/internal/domain"
	"pdconsole/internal/persistence"
)

func TestSealedEvidenceMutationRejectedOnReopen(t *testing.T) {
	directory := t.TempDir()
	store, err := persistence.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	service := application.NewService(store)
	batch, err := service.CreateBatch(application.CreateBatchCommand{
		IdempotencyKey: "create", CableSection: "甲段", CircuitName: "一回", TestOwner: "负责人",
	})
	if err != nil {
		t.Fatal(err)
	}
	points := []domain.TestPoint{{
		ID: "P1", Name: "终端", Location: "甲相终端", SensorRangePC: 100,
		AmplitudeLimitPC: 20, TrendLimitPercent: 25, RepeatabilityCount: 3,
	}}
	preflight, err := service.PreflightScope(points)
	if err != nil {
		t.Fatal(err)
	}
	batch, err = service.FreezeScope(batch.ID, application.FreezeScopeCommand{
		CommandMeta: application.CommandMeta{IdempotencyKey: "freeze", ExpectedVersion: batch.Version, Actor: "负责人"},
		Points:      points, PreflightScopeDigest: preflight.ScopeDigest, Confirmed: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	batch, err = service.AddMeasurements(batch.ID, application.AddMeasurementsCommand{
		CommandMeta: application.CommandMeta{IdempotencyKey: "measure", ExpectedVersion: batch.Version, Actor: "操作员"},
		Measurements: []application.MeasurementInput{{
			ID: "m1", PointID: "P1", Round: 1, MeasuredAt: batch.FrozenScope.FrozenAt.Add(time.Second), PeakAmplitudePC: 8,
			PhaseSummary: "分布均匀", TemperatureC: 25, HumidityPercent: 50, SensorSerial: "S1",
			Operator: "操作员", Purpose: "initial",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	batch, err = service.Diagnose(batch.ID, application.DiagnoseCommand{CommandMeta: application.CommandMeta{
		IdempotencyKey: "diagnose", ExpectedVersion: batch.Version, Actor: "诊断员",
	}})
	if err != nil {
		t.Fatal(err)
	}
	readiness, err := service.GetReviewReadiness(batch.ID)
	if err != nil || !readiness.Ready || readiness.Snapshot == nil {
		t.Fatalf("复核未就绪: %#v, %v", readiness, err)
	}
	for index, reviewer := range []string{"复核甲", "复核乙"} {
		batch, err = service.SubmitReview(batch.ID, application.SubmitReviewCommand{
			CommandMeta: application.CommandMeta{IdempotencyKey: "review-" + reviewer, ExpectedVersion: batch.Version, Actor: reviewer},
			Reviewer:    reviewer, Role: "安全专家", Approved: true, Opinion: "同意复归",
			EvidenceDigest: readiness.Snapshot.Digest, EvidenceVersion: readiness.Snapshot.BatchVersion,
		})
		if err != nil {
			t.Fatalf("第 %d 次复核失败: %v", index+1, err)
		}
	}
	batch, err = service.IssueCertificate(batch.ID, application.IssueCertificateCommand{CommandMeta: application.CommandMeta{
		IdempotencyKey: "issue", ExpectedVersion: batch.Version, Actor: "签发员",
	}})
	if err != nil {
		t.Fatal(err)
	}

	snapshotPath := filepath.Join(directory, "snapshot.json")
	encoded, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot persistence.Snapshot
	if err := json.Unmarshal(encoded, &snapshot); err != nil {
		t.Fatal(err)
	}
	snapshot.Batches[batch.ID].Measurements[0].PeakAmplitudePC = 80
	encoded, err = json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(snapshotPath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}

	reopened, err := persistence.Open(directory)
	if err == nil {
		verification, verifyErr := application.NewService(reopened).VerifyCertificate(batch.ID)
		if verifyErr == nil && verification.Valid {
			t.Fatal("测量证据内容被篡改后，重启校验和在线核验仍同时判定有效")
		}
		t.Fatalf("测量证据内容被篡改后应在重启时拒绝加载，在线核验结果: %#v, error=%v", verification, verifyErr)
	}
}
