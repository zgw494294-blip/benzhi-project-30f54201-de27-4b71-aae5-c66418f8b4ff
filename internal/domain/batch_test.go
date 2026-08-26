package domain

import (
	"errors"
	"testing"
	"time"
)

func TestBatchWorkflowWithDeviationAndSeal(t *testing.T) {
	now := time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC)
	batch, err := NewBatch("batch-1", "北港线 A 段", "北港一回", "张工", now)
	if err != nil {
		t.Fatal(err)
	}
	points := []TestPoint{{ID: "P1", Name: "终端", Location: "A 相终端", SensorRangePC: 100, AmplitudeLimitPC: 20, TrendLimitPercent: 25, RepeatabilityCount: 3}}
	if err := batch.Freeze(points, "张工", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	measurement := Measurement{
		ID: "m1", BatchID: batch.ID, PointID: "P1", Round: 1, MeasuredAt: now.Add(90 * time.Second),
		PeakAmplitudePC: 35, PhaseSummary: "集中尖峰", TemperatureC: 24,
		HumidityPercent: 50, SensorSerial: "S1", Operator: "李工", Purpose: "initial",
	}
	if err := batch.AddMeasurement(measurement, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	created, err := batch.RunDiagnosis(func() string { return "d1" }, now.Add(3*time.Minute))
	if err != nil || len(created) != 1 || created[0].RuleCode != "PD_AMPLITUDE" {
		t.Fatalf("诊断结果异常: %#v, %v", created, err)
	}
	correction := Correction{Measure: "重新处理终端屏蔽层", Assignee: "王工", RetestPoints: []string{"P1"}, RecordedBy: "李工"}
	if err := batch.RecordCorrection("d1", correction, now.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	retest := measurement
	retest.ID = "m2"
	retest.Round = 2
	retest.MeasuredAt = now.Add(5 * time.Minute)
	retest.PeakAmplitudePC = 8
	retest.Purpose = "retest"
	if err := batch.AddMeasurement(retest, now.Add(5*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if result, err := batch.EvaluateRetest("d1", "m2", "读数恢复稳定", now.Add(6*time.Minute)); err != nil || result != RetestPassed {
		t.Fatalf("复验异常: %s, %v", result, err)
	}
	for _, reviewer := range []string{"赵专家", "钱专家"} {
		if err := batch.AddReview(SafetyReview{Reviewer: reviewer, Role: "安全专家", Approved: true, Opinion: "同意"}, now.Add(7*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	certificate, err := batch.IssueCertificate("cert-1", now.Add(8*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if !certificate.Verify() || batch.Status != StatusSealed {
		t.Fatal("证书摘要或封存状态不正确")
	}
	if err := batch.AddReview(SafetyReview{}, now); !errors.Is(err, ErrBatchSealed) {
		t.Fatalf("封存后应拒绝变更，得到 %v", err)
	}
}

func TestDuplicateReviewerRejected(t *testing.T) {
	now := time.Now().UTC()
	batch, _ := NewBatch("b", "区段", "回路", "负责人", now)
	batch.Status = StatusReviewing
	review := SafetyReview{Reviewer: "同一人", Role: "专家", Approved: true, Opinion: "同意"}
	if err := batch.AddReview(review, now); err != nil {
		t.Fatal(err)
	}
	if err := batch.AddReview(review, now); err == nil {
		t.Fatal("应拒绝同一人重复复核")
	}
}

func TestScopeValidation(t *testing.T) {
	bad := []TestPoint{{ID: "P", Name: "点", Location: "位置", SensorRangePC: 10, AmplitudeLimitPC: 20, TrendLimitPercent: 10, RepeatabilityCount: 2}}
	if err := ValidatePoints(bad); err == nil {
		t.Fatal("阈值超过量程应校验失败")
	}
}

func TestScopePreflightStableAndAggregatesRows(t *testing.T) {
	points := []TestPoint{
		{ID: "P2", Name: "接头", Location: "中间接头", SensorRangePC: 100, AmplitudeLimitPC: 20, TrendLimitPercent: 30, RepeatabilityCount: 3},
		{ID: "P1", Name: "终端", Location: "A 相终端", SensorRangePC: 80, AmplitudeLimitPC: 15, TrendLimitPercent: 20, RepeatabilityCount: 2},
	}
	first, err := PreflightScope(points)
	if err != nil {
		t.Fatal(err)
	}
	second, err := PreflightScope([]TestPoint{points[1], points[0]})
	if err != nil {
		t.Fatal(err)
	}
	if first.ScopeDigest != second.ScopeDigest || first.RangeSummary != second.RangeSummary || first.Points[0].ID != "P1" {
		t.Fatal("页面行序变化不应影响规范摘要")
	}
	bad := append([]TestPoint(nil), points...)
	bad[1].ID, bad[1].Location, bad[1].AmplitudeLimitPC = "P2", "中间接头", 200
	_, err = PreflightScope(bad)
	var failures ValidationErrors
	if !errors.As(err, &failures) || len(failures) < 3 {
		t.Fatalf("应同时返回重复编号、位置和量程错误: %#v", err)
	}
}

func TestMeasurementBatchAtomicAndDiagnosisReportReuse(t *testing.T) {
	now := time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC)
	batch, _ := NewBatch("b", "区段", "回路", "负责人", now)
	if err := batch.Freeze([]TestPoint{{ID: "P1", Name: "终端", Location: "位置", SensorRangePC: 100, AmplitudeLimitPC: 20, TrendLimitPercent: 25, RepeatabilityCount: 3}}, "负责人", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	version := batch.Version
	base := Measurement{BatchID: batch.ID, PointID: "P1", MeasuredAt: now.Add(2 * time.Minute), PeakAmplitudePC: 8, PhaseSummary: "均匀", TemperatureC: 25, HumidityPercent: 50, SensorSerial: "S1", Operator: "操作员", Purpose: "initial"}
	first, second := base, base
	first.ID, first.Round = "m1", 1
	second.ID, second.Round, second.PeakAmplitudePC = "m2", 1, 120
	if _, err := batch.AddMeasurements([]Measurement{first, second}, now.Add(3*time.Minute)); err == nil {
		t.Fatal("跨行错误应拒绝整批")
	}
	if batch.Version != version || len(batch.Measurements) != 0 {
		t.Fatal("失败批量不得改变版本或保存部分记录")
	}
	second.Round, second.PeakAmplitudePC = 2, 9
	third := base
	third.ID, third.Round, third.MeasuredAt = "m3", 3, now.Add(4*time.Minute)
	if _, err := batch.AddMeasurements([]Measurement{first, second, third}, now.Add(5*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if batch.Version != version+1 || len(batch.Measurements) != 3 {
		t.Fatal("合法整批应只递增一次版本")
	}
	report, err := batch.RunDiagnosisReport("run-1", func() string { return "d" }, now.Add(6*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	diagnosedVersion := batch.Version
	reused, err := batch.RunDiagnosisReport("run-2", func() string { return "unexpected" }, now.Add(7*time.Minute))
	if err != nil || reused.RunID != report.RunID || batch.Version != diagnosedVersion || len(batch.DiagnosisReports) != 1 {
		t.Fatal("相同证据应复用原诊断报告且不推进版本")
	}
}
