package application

import (
	"errors"
	"fmt"
)

var (
	ErrVersionConflict = errors.New("批次版本冲突")
	ErrIdempotencyKey  = errors.New("缺少幂等键")
)

type DuplicateBatchError struct {
	Matches []BatchSummary `json:"matches"`
}

func (e DuplicateBatchError) Error() string {
	if len(e.Matches) == 0 {
		return "存在未封存的同回路批次"
	}
	return fmt.Sprintf("发现同区段同回路的未封存批次 %s（%s），请确认后继续创建", e.Matches[0].ID, e.Matches[0].StatusLabel)
}

type VersionConflictError struct {
	BatchID string `json:"batchID"`
	Want    int64  `json:"expectedVersion"`
	Actual  int64  `json:"actualVersion"`
}

func (e VersionConflictError) Error() string {
	return fmt.Sprintf("%s: 批次 %s 期望版本 %d，当前版本 %d", ErrVersionConflict, e.BatchID, e.Want, e.Actual)
}

func (e VersionConflictError) Unwrap() error { return ErrVersionConflict }
