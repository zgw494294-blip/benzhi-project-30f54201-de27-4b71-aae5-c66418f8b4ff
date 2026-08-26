package domain

type BatchStatus string

const (
	StatusDraft      BatchStatus = "draft"
	StatusFrozen     BatchStatus = "frozen"
	StatusDiagnosed  BatchStatus = "diagnosed"
	StatusCorrecting BatchStatus = "correcting"
	StatusRetesting  BatchStatus = "retesting"
	StatusReviewing  BatchStatus = "reviewing"
	StatusSealed     BatchStatus = "sealed"
)

func (s BatchStatus) Label() string {
	switch s {
	case StatusDraft:
		return "待冻结"
	case StatusFrozen:
		return "待采集"
	case StatusDiagnosed:
		return "已诊断"
	case StatusCorrecting:
		return "整改中"
	case StatusRetesting:
		return "复验中"
	case StatusReviewing:
		return "安全复核中"
	case StatusSealed:
		return "已复归放行"
	default:
		return "未知状态"
	}
}

func (s BatchStatus) IsMutable() bool { return s != StatusSealed }

type Severity string

const (
	SeverityNotice Severity = "notice"
	SeverityMajor  Severity = "major"
	SeveritySevere Severity = "severe"
)

type RetestResult string

const (
	RetestPending RetestResult = "pending"
	RetestPassed  RetestResult = "passed"
	RetestFailed  RetestResult = "failed"
)
