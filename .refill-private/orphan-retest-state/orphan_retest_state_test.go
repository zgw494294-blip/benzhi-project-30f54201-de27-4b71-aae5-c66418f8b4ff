package orphan_retest_state_test

import (
	"testing"
	"time"

	"pdconsole/internal/application"
	"pdconsole/internal/domain"
	"pdconsole/internal/persistence"
)

func TestRetestRequiresActiveCorrectionTask(t *testing.T) {
	store, err := persistence.Open(t.TempDir())
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
	version := batch.Version
	_, err = service.AddMeasurements(batch.ID, application.AddMeasurementsCommand{
		CommandMeta: application.CommandMeta{IdempotencyKey: "orphan-retest", ExpectedVersion: version, Actor: "操作员"},
		Measurements: []application.MeasurementInput{{
			ID: "retest-before-correction", PointID: "P1", Round: 1, MeasuredAt: batch.FrozenScope.FrozenAt.Add(time.Second),
			PeakAmplitudePC: 8, PhaseSummary: "分布均匀", TemperatureC: 25, HumidityPercent: 50,
			SensorSerial: "S1", Operator: "操作员", Purpose: "retest",
		}},
	})
	if err == nil {
		t.Fatal("不存在已登记整改和复验任务时应拒绝 retest 读数")
	}
	restored, getErr := store.GetBatch(batch.ID)
	if getErr != nil {
		t.Fatal(getErr)
	}
	if restored.Version != version || restored.Status != domain.StatusFrozen || len(restored.Measurements) != 0 {
		t.Fatalf("失败写入不得污染批次状态: version=%d status=%s measurements=%d", restored.Version, restored.Status, len(restored.Measurements))
	}
}
