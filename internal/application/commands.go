package application

import (
	"time"

	"pdconsole/internal/domain"
)

type CommandMeta struct {
	IdempotencyKey  string `json:"idempotencyKey"`
	ExpectedVersion int64  `json:"expectedVersion"`
	Actor           string `json:"actor"`
}

type CreateBatchCommand struct {
	IdempotencyKey   string `json:"idempotencyKey"`
	CableSection     string `json:"cableSection"`
	CircuitName      string `json:"circuitName"`
	TestOwner        string `json:"testOwner"`
	SourceBatchID    string `json:"sourceBatchID"`
	ConfirmDuplicate bool   `json:"confirmDuplicate"`
}

type FreezeScopeCommand struct {
	CommandMeta
	Points               []domain.TestPoint `json:"points"`
	PreflightScopeDigest string             `json:"preflightScopeDigest"`
	Confirmed            bool               `json:"confirmed"`
}

type AddMeasurementCommand struct {
	CommandMeta
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

type MeasurementInput struct {
	ID              string    `json:"id"`
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

type AddMeasurementsCommand struct {
	CommandMeta
	Measurements []MeasurementInput `json:"measurements"`
}

type DiagnoseCommand struct{ CommandMeta }

type CorrectDeviationCommand struct {
	CommandMeta
	Measure      string   `json:"measure"`
	Assignee     string   `json:"assignee"`
	RetestPoints []string `json:"retestPoints"`
}

type EvaluateRetestCommand struct {
	CommandMeta
	MeasurementID  string   `json:"measurementID"`
	MeasurementIDs []string `json:"measurementIDs"`
	Conclusion     string   `json:"conclusion"`
}

type SubmitReviewCommand struct {
	CommandMeta
	Reviewer        string `json:"reviewer"`
	Role            string `json:"role"`
	Approved        bool   `json:"approved"`
	Opinion         string `json:"opinion"`
	EvidenceDigest  string `json:"evidenceDigest"`
	EvidenceVersion int64  `json:"evidenceVersion"`
}

type IssueCertificateCommand struct{ CommandMeta }
