package domain

import (
	"fmt"
	"strings"
	"time"
)

type Measurement struct {
	ID              string    `json:"id"`
	BatchID         string    `json:"batchID"`
	PointID         string    `json:"pointID"`
	Round           int       `json:"round"`
	MeasuredAt      time.Time `json:"measuredAt"`
	PeakAmplitudePC float64   `json:"peakAmplitudePC"`
	PhaseSummary    string    `json:"phaseSummary"`
	TemperatureC    float64   `json:"temperatureC"`
	HumidityPercent float64   `json:"humidityPercent"`
	SensorSerial    string    `json:"sensorSerial"`
	Operator        string    `json:"operator"`
	Purpose         string    `json:"purpose"`
}

func (m *Measurement) NormalizeAndValidate() error {
	m.ID = strings.TrimSpace(m.ID)
	m.BatchID = strings.TrimSpace(m.BatchID)
	m.PointID = strings.TrimSpace(m.PointID)
	m.PhaseSummary = strings.TrimSpace(m.PhaseSummary)
	m.SensorSerial = strings.TrimSpace(m.SensorSerial)
	m.Operator = strings.TrimSpace(m.Operator)
	if m.ID == "" || m.BatchID == "" || m.PointID == "" {
		return invalid("measurement", "记录编号、批次编号和试验点不能为空")
	}
	if m.Round <= 0 {
		return invalid("round", "采样轮次必须大于 0")
	}
	if m.MeasuredAt.IsZero() {
		return invalid("measuredAt", "采样时间不能为空")
	}
	if m.PeakAmplitudePC < 0 {
		return invalid("peakAmplitudePC", "峰值幅度不能为负数")
	}
	if m.PhaseSummary == "" {
		return invalid("phaseSummary", "相位分布摘要不能为空")
	}
	if m.TemperatureC < -50 || m.TemperatureC > 100 {
		return invalid("temperatureC", "温度超出允许范围")
	}
	if m.HumidityPercent < 0 || m.HumidityPercent > 100 {
		return invalid("humidityPercent", "湿度应在 0 到 100 之间")
	}
	if m.SensorSerial == "" || m.Operator == "" {
		return invalid("sensorSerial", "传感器序列号和录入人员不能为空")
	}
	m.Purpose = strings.TrimSpace(m.Purpose)
	if m.Purpose == "" {
		m.Purpose = "initial"
	}
	if m.Purpose != "initial" && m.Purpose != "retest" {
		return invalid("purpose", "采样用途只能是 initial 或 retest")
	}
	return nil
}

type MeasurementBatchSummary struct {
	Count       int               `json:"count"`
	PointCounts map[string]int    `json:"pointCounts"`
	RoundRanges map[string]string `json:"roundRanges"`
	Warnings    []string          `json:"warnings"`
}

// AddMeasurements 先在副本中完成逐行和跨行校验，全部通过后才一次更新聚合版本。
func (b *Batch) AddMeasurements(measurements []Measurement, now time.Time) (MeasurementBatchSummary, error) {
	if err := b.ensureMutable(); err != nil {
		return MeasurementBatchSummary{}, err
	}
	if b.Status == StatusDraft || len(b.Reviews) > 0 {
		return MeasurementBatchSummary{}, ErrInvalidTransition
	}
	if len(measurements) == 0 {
		return MeasurementBatchSummary{}, ValidationErrors{{Field: "measurements", Message: "至少提交一行读数"}}
	}
	allIDs := make(map[string]bool, len(b.Measurements)+len(measurements))
	rounds := make(map[string]bool, len(b.Measurements)+len(measurements))
	for _, existing := range b.Measurements {
		allIDs[existing.ID] = true
		rounds[existing.PointID+"\x00"+existing.Purpose+"\x00"+fmt.Sprint(existing.Round)] = true
	}
	normalized := append([]Measurement(nil), measurements...)
	var failures ValidationErrors
	summary := MeasurementBatchSummary{PointCounts: map[string]int{}, RoundRanges: map[string]string{}}
	minRounds := map[string]int{}
	maxRounds := map[string]int{}
	for index := range normalized {
		measurement := &normalized[index]
		if err := measurement.NormalizeAndValidate(); err != nil {
			if field, ok := err.(FieldError); ok {
				failures = append(failures, FieldError{Field: fmt.Sprintf("measurements[%d].%s", index, field.Field), Message: field.Message})
			} else {
				failures = append(failures, FieldError{Field: fmt.Sprintf("measurements[%d]", index), Message: err.Error()})
			}
			continue
		}
		if measurement.BatchID != b.ID {
			failures = append(failures, FieldError{Field: fmt.Sprintf("measurements[%d].batchID", index), Message: "测量记录不属于当前批次"})
		}
		point, ok := pointByID(b.FrozenScope, measurement.PointID)
		if !ok {
			failures = append(failures, FieldError{Field: fmt.Sprintf("measurements[%d].pointID", index), Message: "试验点不在冻结范围内"})
		} else if measurement.PeakAmplitudePC > point.SensorRangePC {
			failures = append(failures, FieldError{Field: fmt.Sprintf("measurements[%d].peakAmplitudePC", index), Message: "峰值幅度超过冻结的传感器量程"})
		}
		if measurement.MeasuredAt.Before(b.FrozenScope.FrozenAt) {
			failures = append(failures, FieldError{Field: fmt.Sprintf("measurements[%d].measuredAt", index), Message: "采样时间不得早于边界冻结时间"})
		}
		if measurement.MeasuredAt.After(now.UTC().Add(5 * time.Minute)) {
			failures = append(failures, FieldError{Field: fmt.Sprintf("measurements[%d].measuredAt", index), Message: "采样时间显著晚于当前时间"})
		}
		if allIDs[measurement.ID] {
			failures = append(failures, FieldError{Field: fmt.Sprintf("measurements[%d].id", index), Message: "记录编号在批内或历史记录中重复"})
		} else {
			allIDs[measurement.ID] = true
		}
		roundKey := measurement.PointID + "\x00" + measurement.Purpose + "\x00" + fmt.Sprint(measurement.Round)
		if rounds[roundKey] {
			failures = append(failures, FieldError{Field: fmt.Sprintf("measurements[%d].round", index), Message: "同一试验点和采样用途的轮次不能重复"})
		} else {
			rounds[roundKey] = true
		}
		if measurement.Purpose == "retest" && !b.hasOpenRetestTaskForPoint(measurement.PointID) {
			failures = append(failures, FieldError{Field: fmt.Sprintf("measurements[%d].purpose", index), Message: "复验读数必须对应已登记整改的未关闭偏差"})
		}
		summary.PointCounts[measurement.PointID]++
		key := measurement.PointID + "/" + measurement.Purpose
		if minRounds[key] == 0 || measurement.Round < minRounds[key] {
			minRounds[key] = measurement.Round
		}
		if measurement.Round > maxRounds[key] {
			maxRounds[key] = measurement.Round
		}
		if ok && measurement.PeakAmplitudePC > point.AmplitudeLimitPC {
			summary.Warnings = append(summary.Warnings, fmt.Sprintf("第 %d 行峰值超过 %s 的幅值阈值", index+1, point.ID))
		}
	}
	if len(failures) > 0 {
		return MeasurementBatchSummary{}, failures
	}
	for key, minimum := range minRounds {
		summary.RoundRanges[key] = fmt.Sprintf("%d-%d", minimum, maxRounds[key])
	}
	summary.Count = len(normalized)
	b.Measurements = append(b.Measurements, normalized...)
	hasRetest := false
	for _, measurement := range normalized {
		if measurement.Purpose == "retest" {
			hasRetest = true
		}
	}
	if hasRetest {
		b.Status = StatusRetesting
	} else if b.OpenDeviationCount() == 0 {
		b.Status = StatusFrozen
	} else if b.Status != StatusCorrecting && b.Status != StatusRetesting {
		b.Status = StatusDiagnosed
	}
	b.touch(now)
	return summary, nil
}
