package domain

import (
	"fmt"
	"strings"
	"time"
)

type Batch struct {
	ID                  string                  `json:"id"`
	SourceBatchID       string                  `json:"sourceBatchID,omitempty"`
	CableSection        string                  `json:"cableSection"`
	CircuitName         string                  `json:"circuitName"`
	TestOwner           string                  `json:"testOwner"`
	Status              BatchStatus             `json:"status"`
	Version             int64                   `json:"version"`
	FrozenScope         FrozenScope             `json:"frozenScope"`
	Measurements        []Measurement           `json:"measurements"`
	Deviations          []Deviation             `json:"deviations"`
	Reviews             []SafetyReview          `json:"reviews"`
	Certificate         *Certificate            `json:"certificate,omitempty"`
	DiagnosisReports    []DiagnosisReport       `json:"diagnosisReports"`
	ReviewSnapshot      *ReviewEvidenceSnapshot `json:"reviewSnapshot,omitempty"`
	VerificationPackage *VerificationPackage    `json:"verificationPackage,omitempty"`
	DiagnosisRun        int                     `json:"diagnosisRun"`
	CreatedAt           time.Time               `json:"createdAt"`
	UpdatedAt           time.Time               `json:"updatedAt"`
}

func NewBatch(id, cableSection, circuitName, owner string, now time.Time) (*Batch, error) {
	b := &Batch{
		ID: strings.TrimSpace(id), CableSection: strings.TrimSpace(cableSection),
		CircuitName: strings.TrimSpace(circuitName), TestOwner: strings.TrimSpace(owner),
		Status: StatusDraft, Version: 1, CreatedAt: now.UTC(), UpdatedAt: now.UTC(),
	}
	if b.ID == "" {
		return nil, invalid("id", "批次编号不能为空")
	}
	if b.CableSection == "" {
		return nil, invalid("cableSection", "电缆区段不能为空")
	}
	if b.CircuitName == "" {
		return nil, invalid("circuitName", "回路名称不能为空")
	}
	if b.TestOwner == "" {
		return nil, invalid("testOwner", "试验负责人不能为空")
	}
	return b, nil
}

func NewBatchFromSource(id string, source *Batch, now time.Time) (*Batch, error) {
	if source == nil || strings.TrimSpace(source.ID) == "" {
		return nil, invalid("sourceBatchID", "来源批次不存在")
	}
	batch, err := NewBatch(id, source.CableSection, source.CircuitName, source.TestOwner, now)
	if err != nil {
		return nil, err
	}
	batch.SourceBatchID = source.ID
	return batch, nil
}

func (b *Batch) ensureMutable() error {
	if b.Status == StatusSealed || b.Certificate != nil {
		return ErrBatchSealed
	}
	return nil
}

func (b *Batch) touch(now time.Time) {
	b.Version++
	b.UpdatedAt = now.UTC()
}

func (b *Batch) Freeze(points []TestPoint, actor string, now time.Time) error {
	if err := b.ensureMutable(); err != nil {
		return err
	}
	if b.Status != StatusDraft {
		return ErrScopeFrozen
	}
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return invalid("frozenBy", "冻结操作人不能为空")
	}
	preflight, err := PreflightScope(points)
	if err != nil {
		return err
	}
	b.FrozenScope = FrozenScope{Points: preflight.Points, FrozenBy: actor, FrozenAt: now.UTC(), ScopeDigest: preflight.ScopeDigest, RangeSummary: preflight.RangeSummary}
	b.Status = StatusFrozen
	b.touch(now)
	return nil
}

func (b *Batch) AddMeasurement(measurement Measurement, now time.Time) error {
	_, err := b.AddMeasurements([]Measurement{measurement}, now)
	return err
}

func (b *Batch) OpenDeviationCount() int {
	count := 0
	for _, deviation := range b.Deviations {
		if !deviation.IsClosed() {
			count++
		}
	}
	return count
}

// hasOpenRetestTaskForPoint 报告是否存在未关闭、已登记整改且复验范围包含 pointID 的偏差，
// 用于在校验复验读数时确认具备复验前提。
func (b *Batch) hasOpenRetestTaskForPoint(pointID string) bool {
	for _, deviation := range b.Deviations {
		if deviation.IsClosed() {
			continue
		}
		if deviation.Correction == nil || deviation.RetestTask == nil {
			continue
		}
		if deviation.PointID != pointID {
			continue
		}
		for _, id := range deviation.Correction.RetestPoints {
			if id == pointID {
				return true
			}
		}
	}
	return false
}

func (b *Batch) FindDeviation(id string) (*Deviation, error) {
	for i := range b.Deviations {
		if b.Deviations[i].ID == id {
			return &b.Deviations[i], nil
		}
	}
	return nil, fmt.Errorf("%w: 偏差 %s", ErrNotFound, id)
}

func (b *Batch) RecordCorrection(deviationID string, correction Correction, now time.Time) error {
	if err := b.ensureMutable(); err != nil {
		return err
	}
	if b.Status != StatusDiagnosed && b.Status != StatusCorrecting && b.Status != StatusRetesting {
		return ErrInvalidTransition
	}
	deviation, err := b.FindDeviation(deviationID)
	if err != nil {
		return err
	}
	if deviation.IsClosed() {
		return invalid("deviationID", "已关闭偏差无需重复整改")
	}
	correction.RecordedAt = now.UTC()
	if err := deviation.SetCorrection(correction); err != nil {
		return err
	}
	point, _ := pointByID(b.FrozenScope, deviation.PointID)
	required, metric, limit := 1, "单轮限值", fmt.Sprintf("幅值 ≤ %.2f pC", point.AmplitudeLimitPC)
	switch deviation.RuleCode {
	case "PD_TREND":
		required, metric, limit = 2, "首末轮幅值涨幅", fmt.Sprintf("涨幅 ≤ %.1f%%", point.TrendLimitPercent)
	case "PD_REPEAT":
		required, metric, limit = point.RepeatabilityCount, "接近阈值的重复轮次", fmt.Sprintf("少于 %d 轮达到阈值 90%%", point.RepeatabilityCount)
	case "PHASE_CLUSTER":
		required, metric, limit = point.RepeatabilityCount, "相位集中尖峰重复次数", fmt.Sprintf("少于 %d 轮集中尖峰", point.RepeatabilityCount)
	case "ENV_CONDITION":
		metric, limit = "温湿度单轮限值", "温度 -10~55 ℃且湿度 ≤ 80%"
	}
	deviation.RetestTask = &RetestTask{RuleCode: deviation.RuleCode, PointID: deviation.PointID,
		RequiredRounds: required, Metric: metric, FrozenLimit: limit, MissingRounds: required, Status: "pending"}
	b.Status = StatusCorrecting
	b.touch(now)
	return nil
}
