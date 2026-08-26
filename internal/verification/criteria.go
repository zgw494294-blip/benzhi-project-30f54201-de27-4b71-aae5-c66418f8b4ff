package verification

import (
	"fmt"

	"isolation-chamber-commissioning/internal/domain"
)

func evaluate(kind domain.CheckpointKind, l domain.AcceptanceLimits, m []domain.Measurement) []string {
	switch kind {
	case domain.KindPressure:
		return pressure(l, m)
	case domain.KindAirtightness:
		return airtightness(l, m)
	case domain.KindInterlock:
		return interlock(l, m)
	case domain.KindRecovery:
		return recovery(l, m)
	default:
		return []string{"未知检查点类型"}
	}
}

func criterion(code, label, comparator string, limit, actual float64, unit string, passed bool) domain.CriterionEvidence {
	result := "不满足"
	if passed {
		result = "满足"
	}
	return domain.CriterionEvidence{Code: code, Label: label, Comparator: comparator, Limit: limit, Actual: actual, Unit: unit, Passed: passed, Explanation: fmt.Sprintf("实测 %.2f %s %s 限值 %.2f %s：%s", actual, unit, comparator, limit, unit, result)}
}

func missingCriterion(code, label, comparator string, limit float64, unit string) domain.CriterionEvidence {
	return domain.CriterionEvidence{Code: code, Label: label, Comparator: comparator, Limit: limit, Unit: unit, Passed: false, Explanation: "缺少判定所需的原始测量"}
}

func buildCriteria(kind domain.CheckpointKind, l domain.AcceptanceLimits, m []domain.Measurement) []domain.CriterionEvidence {
	switch kind {
	case domain.KindPressure:
		samples := measurementsByName(m, "pressurePa")
		if len(samples) == 0 {
			return []domain.CriterionEvidence{missingCriterion("PRESSURE_MIN", "持续窗口最低压差", ">=", l.PressureMinPa, "Pa"), missingCriterion("PRESSURE_WINDOW", "压差采样持续窗口", ">=", float64(l.PressureDurationSec), "s")}
		}
		minimum := samples[0].Value
		first, last := samples[0].OffsetSec, samples[0].OffsetSec
		for _, sample := range samples {
			if sample.Value < minimum {
				minimum = sample.Value
			}
			if sample.OffsetSec < first {
				first = sample.OffsetSec
			}
			if sample.OffsetSec > last {
				last = sample.OffsetSec
			}
		}
		window := float64(last - first)
		return []domain.CriterionEvidence{
			criterion("PRESSURE_MIN", "持续窗口最低压差", ">=", l.PressureMinPa, minimum, "Pa", minimum >= l.PressureMinPa),
			criterion("PRESSURE_WINDOW", "压差采样持续窗口", ">=", float64(l.PressureDurationSec), window, "s", window >= float64(l.PressureDurationSec)),
		}
	case domain.KindAirtightness:
		value, ok := require(m, "leakagePercent")
		if !ok {
			return []domain.CriterionEvidence{missingCriterion("LEAKAGE_MAX", "最大泄漏率", "<=", l.MaxLeakagePercent, "%")}
		}
		return []domain.CriterionEvidence{criterion("LEAKAGE_MAX", "最大泄漏率", "<=", l.MaxLeakagePercent, value.Value, "%", value.Value <= l.MaxLeakagePercent)}
	case domain.KindInterlock:
		out := make([]domain.CriterionEvidence, 0, 2)
		response, ok := require(m, "responseSec")
		if ok {
			out = append(out, criterion("INTERLOCK_RESPONSE", "互锁响应时间", "<=", l.InterlockResponseSec, response.Value, "s", response.Value <= l.InterlockResponseSec))
		} else {
			out = append(out, missingCriterion("INTERLOCK_RESPONSE", "互锁响应时间", "<=", l.InterlockResponseSec, "s"))
		}
		doors, ok := require(m, "bothDoorsOpen")
		if !ok || doors.Flag == nil {
			out = append(out, missingCriterion("INTERLOCK_EXCLUSION", "双门互斥", "=", 0, "boolean"))
		} else {
			actual := 0.0
			if *doors.Flag {
				actual = 1
			}
			out = append(out, criterion("INTERLOCK_EXCLUSION", "双门互斥", "=", 0, actual, "boolean", !*doors.Flag))
		}
		return out
	case domain.KindRecovery:
		out := make([]domain.CriterionEvidence, 0, 2)
		minutes, ok := require(m, "recoveryMinutes")
		if ok {
			out = append(out, criterion("RECOVERY_TIME", "净化恢复时间", "<=", l.RecoveryMaxMinutes, minutes.Value, "min", minutes.Value <= l.RecoveryMaxMinutes))
		} else {
			out = append(out, missingCriterion("RECOVERY_TIME", "净化恢复时间", "<=", l.RecoveryMaxMinutes, "min"))
		}
		particles, ok := require(m, "particleCount")
		if ok {
			out = append(out, criterion("RECOVERY_PARTICLES", "恢复终点粒子浓度", "<=", l.RecoveryTargetParticles, particles.Value, "particles/m3", particles.Value <= l.RecoveryTargetParticles))
		} else {
			out = append(out, missingCriterion("RECOVERY_PARTICLES", "恢复终点粒子浓度", "<=", l.RecoveryTargetParticles, "particles/m3"))
		}
		return out
	default:
		return []domain.CriterionEvidence{{Code: "UNKNOWN_KIND", Label: "检查点类型", Passed: false, Explanation: "规则集不识别该检查点类型"}}
	}
}

func pressure(l domain.AcceptanceLimits, m []domain.Measurement) []string {
	samples := measurementsByName(m, "pressurePa")
	if len(samples) == 0 {
		return []string{"缺少 pressurePa 压差样本"}
	}
	var reasons []string
	min := samples[0].Value
	first := samples[0].OffsetSec
	last := first
	for _, s := range samples {
		if s.Value < min {
			min = s.Value
		}
		if s.OffsetSec < first {
			first = s.OffsetSec
		}
		if s.OffsetSec > last {
			last = s.OffsetSec
		}
	}
	if min < l.PressureMinPa {
		reasons = append(reasons, fmt.Sprintf("最低压差 %.2f Pa 低于限值 %.2f Pa", min, l.PressureMinPa))
	}
	if last-first < l.PressureDurationSec {
		reasons = append(reasons, fmt.Sprintf("有效采样窗口 %d 秒短于要求 %d 秒", last-first, l.PressureDurationSec))
	}
	return reasons
}

func airtightness(l domain.AcceptanceLimits, m []domain.Measurement) []string {
	v, ok := require(m, "leakagePercent")
	if !ok {
		return []string{"缺少 leakagePercent 泄漏率"}
	}
	if v.Value > l.MaxLeakagePercent {
		return []string{fmt.Sprintf("泄漏率 %.2f%% 高于上限 %.2f%%", v.Value, l.MaxLeakagePercent)}
	}
	return nil
}

func interlock(l domain.AcceptanceLimits, m []domain.Measurement) []string {
	var r []string
	v, ok := require(m, "responseSec")
	if !ok {
		r = append(r, "缺少 responseSec 响应时间")
	} else if v.Value > l.InterlockResponseSec {
		r = append(r, fmt.Sprintf("互锁响应 %.2f 秒超过上限 %.2f 秒", v.Value, l.InterlockResponseSec))
	}
	doors, ok := require(m, "bothDoorsOpen")
	if !ok || doors.Flag == nil {
		r = append(r, "缺少 bothDoorsOpen 布尔结果")
	} else if *doors.Flag {
		r = append(r, "测试期间出现双门同时开启")
	}
	return r
}

func recovery(l domain.AcceptanceLimits, m []domain.Measurement) []string {
	var r []string
	minutes, ok := require(m, "recoveryMinutes")
	if !ok {
		r = append(r, "缺少 recoveryMinutes 恢复时间")
	} else if minutes.Value > l.RecoveryMaxMinutes {
		r = append(r, fmt.Sprintf("恢复时间 %.2f 分钟超过上限 %.2f 分钟", minutes.Value, l.RecoveryMaxMinutes))
	}
	particles, ok := require(m, "particleCount")
	if !ok {
		r = append(r, "缺少 particleCount 终点粒子浓度")
	} else if particles.Value > l.RecoveryTargetParticles {
		r = append(r, fmt.Sprintf("终点粒子浓度 %.2f 高于目标 %.2f", particles.Value, l.RecoveryTargetParticles))
	}
	return r
}
