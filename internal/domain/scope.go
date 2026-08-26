package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type TestPoint struct {
	ID                 string  `json:"id"`
	Name               string  `json:"name"`
	Location           string  `json:"location"`
	SensorRangePC      float64 `json:"sensorRangePC"`
	AmplitudeLimitPC   float64 `json:"amplitudeLimitPC"`
	TrendLimitPercent  float64 `json:"trendLimitPercent"`
	RepeatabilityCount int     `json:"repeatabilityCount"`
}

type FrozenScope struct {
	Points       []TestPoint `json:"points"`
	FrozenBy     string      `json:"frozenBy"`
	FrozenAt     time.Time   `json:"frozenAt"`
	ScopeDigest  string      `json:"scopeDigest"`
	RangeSummary string      `json:"rangeSummary"`
}

type ScopePreflight struct {
	Points       []TestPoint `json:"points"`
	ScopeDigest  string      `json:"scopeDigest"`
	RangeSummary string      `json:"rangeSummary"`
}

func ValidatePoints(points []TestPoint) error {
	_, err := PreflightScope(points)
	return err
}

// PreflightScope 聚合所有行错误，并按编号规范排序后计算稳定摘要。
func PreflightScope(points []TestPoint) (ScopePreflight, error) {
	if len(points) == 0 {
		return ScopePreflight{}, ValidationErrors{{Field: "points", Message: "至少配置一个试验点"}}
	}
	normalized := append([]TestPoint(nil), points...)
	seenIDs := make(map[string]int, len(points))
	seenLocations := make(map[string]int, len(points))
	var failures ValidationErrors
	for i := range normalized {
		p := &normalized[i]
		p.ID = strings.TrimSpace(p.ID)
		p.Name = strings.TrimSpace(p.Name)
		p.Location = strings.TrimSpace(p.Location)
		if p.ID == "" {
			failures = append(failures, FieldError{Field: fmt.Sprintf("points[%d].id", i), Message: "试验点编号不能为空"})
		}
		if p.Name == "" {
			failures = append(failures, FieldError{Field: fmt.Sprintf("points[%d].name", i), Message: "试验点名称不能为空"})
		}
		if p.Location == "" {
			failures = append(failures, FieldError{Field: fmt.Sprintf("points[%d].location", i), Message: "物理位置不能为空"})
		}
		idKey := strings.ToLower(p.ID)
		if first, exists := seenIDs[idKey]; p.ID != "" && exists {
			failures = append(failures, FieldError{Field: fmt.Sprintf("points[%d].id", i), Message: fmt.Sprintf("与第 %d 行试验点编号重复", first+1)})
		} else if p.ID != "" {
			seenIDs[idKey] = i
		}
		locationKey := strings.ToLower(p.Location)
		if first, exists := seenLocations[locationKey]; p.Location != "" && exists {
			failures = append(failures, FieldError{Field: fmt.Sprintf("points[%d].location", i), Message: fmt.Sprintf("与第 %d 行物理位置重复", first+1)})
		} else if p.Location != "" {
			seenLocations[locationKey] = i
		}
		if p.SensorRangePC <= 0 {
			failures = append(failures, FieldError{Field: fmt.Sprintf("points[%d].sensorRangePC", i), Message: "传感器量程必须大于 0"})
		}
		if p.AmplitudeLimitPC <= 0 || p.AmplitudeLimitPC > p.SensorRangePC {
			failures = append(failures, FieldError{Field: fmt.Sprintf("points[%d].amplitudeLimitPC", i), Message: "阈值必须大于 0 且不超过量程"})
		}
		if p.TrendLimitPercent <= 0 || p.TrendLimitPercent > 500 {
			failures = append(failures, FieldError{Field: fmt.Sprintf("points[%d].trendLimitPercent", i), Message: "趋势阈值应在 0 到 500 之间"})
		}
		if p.RepeatabilityCount < 2 || p.RepeatabilityCount > 20 {
			failures = append(failures, FieldError{Field: fmt.Sprintf("points[%d].repeatabilityCount", i), Message: "重复性次数应在 2 到 20 之间"})
		}
	}
	if len(failures) > 0 {
		return ScopePreflight{}, failures
	}
	canonical := canonicalPoints(normalized)
	digest, err := digestValue(canonical)
	if err != nil {
		return ScopePreflight{}, err
	}
	parts := make([]string, 0, len(canonical))
	for _, point := range canonical {
		parts = append(parts, fmt.Sprintf("%s@%s:量程%.2fpC/幅值≤%.2fpC/趋势≤%.1f%%/重复%d次", point.ID, point.Location, point.SensorRangePC, point.AmplitudeLimitPC, point.TrendLimitPercent, point.RepeatabilityCount))
	}
	return ScopePreflight{Points: canonical, ScopeDigest: digest, RangeSummary: strings.Join(parts, "；")}, nil
}

func pointByID(scope FrozenScope, id string) (TestPoint, bool) {
	for _, point := range scope.Points {
		if point.ID == id {
			return point, true
		}
	}
	return TestPoint{}, false
}

func canonicalPoints(points []TestPoint) []TestPoint {
	copyPoints := append([]TestPoint(nil), points...)
	sort.Slice(copyPoints, func(i, j int) bool { return copyPoints[i].ID < copyPoints[j].ID })
	return copyPoints
}
