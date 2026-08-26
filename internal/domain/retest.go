package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

func (b *Batch) EvaluateRetest(deviationID, measurementID, conclusion string, now time.Time) (RetestResult, error) {
	return b.EvaluateRetestMeasurements(deviationID, []string{measurementID}, conclusion, now)
}

func (b *Batch) EvaluateRetestMeasurements(deviationID string, measurementIDs []string, conclusion string, now time.Time) (RetestResult, error) {
	if err := b.ensureMutable(); err != nil {
		return RetestPending, err
	}
	if len(b.Reviews) > 0 {
		return RetestPending, ErrReviewLocked
	}
	deviation, err := b.FindDeviation(deviationID)
	if err != nil {
		return RetestPending, err
	}
	if deviation.Correction == nil || deviation.RetestTask == nil {
		return RetestPending, invalid("correction", "登记整改措施后才能复验")
	}
	linked := map[string]bool{}
	for _, id := range deviation.RetestTask.LinkedMeasurementIDs {
		linked[id] = true
	}
	for _, id := range measurementIDs {
		if strings.TrimSpace(id) != "" {
			linked[strings.TrimSpace(id)] = true
		}
	}
	items := make([]Measurement, 0, len(linked))
	for id := range linked {
		var found *Measurement
		for index := range b.Measurements {
			if b.Measurements[index].ID == id {
				found = &b.Measurements[index]
				break
			}
		}
		if found == nil {
			return RetestPending, fmt.Errorf("%w: 复验记录 %s", ErrNotFound, id)
		}
		if found.Purpose != "retest" {
			return RetestPending, invalid("measurementIDs", "只能关联定向复验读数")
		}
		inScope := false
		for _, pointID := range deviation.Correction.RetestPoints {
			if pointID == found.PointID {
				inScope = true
				break
			}
		}
		if !inScope || found.PointID != deviation.PointID {
			return RetestPending, invalid("measurementIDs", "复验读数超出整改范围或不属于偏差点位")
		}
		if !found.MeasuredAt.After(deviation.Correction.RecordedAt) {
			return RetestPending, invalid("measurementIDs", "复验采样时间必须晚于整改登记时间")
		}
		items = append(items, *found)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].MeasuredAt.Equal(items[j].MeasuredAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].MeasuredAt.Before(items[j].MeasuredAt)
	})
	deviation.RetestTask.LinkedMeasurementIDs = evidenceIDs(items)
	missing := deviation.RetestTask.RequiredRounds - len(items)
	if missing < 0 {
		missing = 0
	}
	deviation.RetestTask.MissingRounds = missing
	conclusion = strings.TrimSpace(conclusion)
	if missing > 0 {
		deviation.RetestTask.Status = "pending"
		deviation.RetestTask.FailureReason = fmt.Sprintf("证据不足，还需 %d 轮复验读数", missing)
		deviation.Retest = &RetestEvidence{MeasurementIDs: evidenceIDs(items), Result: RetestPending, Conclusion: deviation.RetestTask.FailureReason, EvaluatedAt: now.UTC()}
		b.Status = StatusRetesting
		b.touch(now)
		return RetestPending, nil
	}
	point, _ := pointByID(b.FrozenScope, deviation.PointID)
	rule := retestRuleResult(deviation.RuleCode, point, items)
	passed := rule.Outcome == RulePassed
	result := RetestFailed
	if passed {
		result = RetestPassed
	}
	if conclusion == "" {
		conclusion = rule.Explanation
	}
	deviation.Retest = &RetestEvidence{MeasurementID: items[len(items)-1].ID, MeasurementIDs: evidenceIDs(items), Result: result, Conclusion: conclusion, EvaluatedAt: now.UTC()}
	if passed {
		closed := now.UTC()
		deviation.ClosedAt = &closed
		deviation.RetestTask.Status = "passed"
		deviation.RetestTask.FailureReason = ""
	} else {
		deviation.ClosedAt = nil
		deviation.RetestTask.Status = "failed"
		deviation.RetestTask.FailureReason = rule.Explanation
	}
	if b.OpenDeviationCount() == 0 {
		b.Status = StatusReviewing
	} else {
		b.Status = StatusCorrecting
	}
	b.touch(now)
	return result, nil
}

func retestRuleResult(ruleCode string, point TestPoint, items []Measurement) RuleResult {
	switch ruleCode {
	case "PD_TREND":
		return evaluateTrend(point, items)
	case "PD_REPEAT":
		return evaluateRepeat(point, items)
	case "PHASE_CLUSTER":
		return evaluatePhase(point, items)
	case "ENV_CONDITION":
		return evaluateEnvironment(point, items)
	default:
		return evaluateAmplitude(point, items)
	}
}
