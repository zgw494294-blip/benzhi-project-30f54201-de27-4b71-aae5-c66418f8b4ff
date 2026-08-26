package domain

import (
	"strings"
	"time"
)

type Correction struct {
	Measure      string    `json:"measure"`
	Assignee     string    `json:"assignee"`
	RetestPoints []string  `json:"retestPoints"`
	RecordedBy   string    `json:"recordedBy"`
	RecordedAt   time.Time `json:"recordedAt"`
}

type RetestEvidence struct {
	MeasurementID  string       `json:"measurementID,omitempty"`
	MeasurementIDs []string     `json:"measurementIDs"`
	Result         RetestResult `json:"result"`
	Conclusion     string       `json:"conclusion"`
	EvaluatedAt    time.Time    `json:"evaluatedAt"`
}

type RetestTask struct {
	RuleCode             string   `json:"ruleCode"`
	PointID              string   `json:"pointID"`
	RequiredRounds       int      `json:"requiredRounds"`
	Metric               string   `json:"metric"`
	FrozenLimit          string   `json:"frozenLimit"`
	LinkedMeasurementIDs []string `json:"linkedMeasurementIDs"`
	MissingRounds        int      `json:"missingRounds"`
	Status               string   `json:"status"`
	FailureReason        string   `json:"failureReason,omitempty"`
}

type Deviation struct {
	ID           string          `json:"id"`
	BatchID      string          `json:"batchID"`
	RuleCode     string          `json:"ruleCode"`
	Severity     Severity        `json:"severity"`
	Location     string          `json:"location"`
	PointID      string          `json:"pointID"`
	Finding      string          `json:"finding"`
	EvidenceIDs  []string        `json:"evidenceIDs"`
	Correction   *Correction     `json:"correction,omitempty"`
	Retest       *RetestEvidence `json:"retest,omitempty"`
	RetestTask   *RetestTask     `json:"retestTask,omitempty"`
	CreatedAt    time.Time       `json:"createdAt"`
	ClosedAt     *time.Time      `json:"closedAt,omitempty"`
	DiagnosisRun int             `json:"diagnosisRun"`
}

func (d Deviation) IsClosed() bool {
	return d.ClosedAt != nil && d.Retest != nil && d.Retest.Result == RetestPassed
}

func (d *Deviation) SetCorrection(c Correction) error {
	c.Measure = strings.TrimSpace(c.Measure)
	c.Assignee = strings.TrimSpace(c.Assignee)
	c.RecordedBy = strings.TrimSpace(c.RecordedBy)
	if c.Measure == "" || c.Assignee == "" || c.RecordedBy == "" {
		return invalid("correction", "整改措施、责任人和登记人不能为空")
	}
	if len(c.RetestPoints) == 0 {
		return invalid("retestPoints", "至少指定一个复验点")
	}
	found := false
	for _, id := range c.RetestPoints {
		if id == d.PointID {
			found = true
		}
	}
	if !found {
		return invalid("retestPoints", "复验范围必须包含偏差所在试验点")
	}
	d.Correction = &c
	d.Retest = nil
	d.ClosedAt = nil
	return nil
}
