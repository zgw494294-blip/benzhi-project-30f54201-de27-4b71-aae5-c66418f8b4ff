package domain

import (
	"strings"
	"time"
)

type SafetyReview struct {
	Reviewer        string    `json:"reviewer"`
	Role            string    `json:"role"`
	Approved        bool      `json:"approved"`
	Opinion         string    `json:"opinion"`
	SubmittedAt     time.Time `json:"submittedAt"`
	EvidenceDigest  string    `json:"evidenceDigest"`
	EvidenceVersion int64     `json:"evidenceVersion"`
	DeviationCount  int       `json:"deviationCount"`
}

func (r *SafetyReview) NormalizeAndValidate() error {
	r.Reviewer = strings.TrimSpace(r.Reviewer)
	r.Role = strings.TrimSpace(r.Role)
	r.Opinion = strings.TrimSpace(r.Opinion)
	if r.Reviewer == "" || r.Role == "" || r.Opinion == "" {
		return invalid("review", "复核人、角色和意见不能为空")
	}
	if r.SubmittedAt.IsZero() {
		return invalid("submittedAt", "复核时间不能为空")
	}
	return nil
}

func reviewsApproved(reviews []SafetyReview) bool {
	if len(reviews) != 2 {
		return false
	}
	return reviews[0].Approved && reviews[1].Approved && reviews[0].Reviewer != reviews[1].Reviewer
}
