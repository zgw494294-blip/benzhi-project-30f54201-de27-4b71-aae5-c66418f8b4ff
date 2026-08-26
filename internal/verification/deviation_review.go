package verification

import (
	"fmt"
	"math"

	"isolation-chamber-commissioning/internal/domain"
)

func AssessDeviation(run domain.TestRun, protocol domain.TestProtocol) (domain.DeviationAssessment, error) {
	scope, err := RetestScope(run, protocol)
	if err != nil {
		return domain.DeviationAssessment{}, err
	}
	level := domain.RiskLow
	basis := []string{fmt.Sprintf("失败规则 %d 项", len(run.FailureReasons))}
	if len(run.FailureReasons) > 1 {
		level = domain.RiskMedium
	}
	for _, criterion := range run.Criteria {
		if criterion.Passed {
			continue
		}
		if criterion.Code == "INTERLOCK_EXCLUSION" {
			level = domain.RiskHigh
			basis = append(basis, "门禁互锁出现双门同开排他失败")
			continue
		}
		if criterion.Limit == 0 {
			continue
		}
		var excess float64
		if criterion.Comparator == "<=" {
			excess = (criterion.Actual - criterion.Limit) / math.Abs(criterion.Limit)
		} else {
			excess = (criterion.Limit - criterion.Actual) / math.Abs(criterion.Limit)
		}
		if excess > 0.5 {
			level = domain.RiskHigh
			basis = append(basis, fmt.Sprintf("%s 超限幅度 %.0f%%", criterion.Code, excess*100))
		} else if excess > 0.2 && level == domain.RiskLow {
			level = domain.RiskMedium
			basis = append(basis, fmt.Sprintf("%s 超限幅度 %.0f%%", criterion.Code, excess*100))
		}
	}
	return domain.DeviationAssessment{SuggestedRisk: level, Basis: basis, Scope: scope, ScopeSource: "由失败证据 " + run.ID + " 的检查点 " + run.CheckpointID + " 计算并锁定"}, nil
}

func ReviewChecklist(c *domain.CommissioningCase) ([]domain.ChecklistItem, error) {
	if c.Protocol == nil {
		return nil, domain.State("案卷没有冻结方案")
	}
	items := []domain.ChecklistItem{{ID: "protocol.summary", Group: "PROTOCOL", Label: "冻结方案版本、检查点顺序与规则版本一致"}}
	latest := map[string]domain.TestRun{}
	for _, run := range c.Runs {
		latest[run.CheckpointID] = run
	}
	for _, cp := range c.Protocol.Checkpoints {
		run, ok := latest[cp.ID]
		if !ok {
			return nil, domain.State("复核清单缺少检查点证据：" + cp.ID)
		}
		items = append(items, domain.ChecklistItem{ID: "evidence." + cp.ID, Group: "EVIDENCE", Label: cp.Name + "最新证据合格且摘要可复算", CheckpointID: cp.ID, EvidenceRefs: []string{run.ID, run.EvidenceDigest}})
	}
	refs := make([]string, 0, len(c.Deviations))
	for _, d := range c.Deviations {
		if d.Status != domain.DeviationClosed {
			return nil, domain.State("仍有偏差未闭环：" + d.ID)
		}
		refs = append(refs, d.ID)
	}
	items = append(items, domain.ChecklistItem{ID: "deviations.closure", Group: "DEVIATIONS", Label: "全部偏差均按锁定范围完成定向复测", EvidenceRefs: refs})
	items = append(items, domain.ChecklistItem{ID: "summary.snapshot", Group: "SUMMARY", Label: "案卷摘要覆盖基础资料、方案、证据和偏差历史"})
	return items, nil
}
