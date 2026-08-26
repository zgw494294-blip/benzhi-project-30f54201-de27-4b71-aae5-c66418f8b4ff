package persistence

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"pdconsole/internal/domain"
)

func emptySnapshot() Snapshot {
	return Snapshot{
		SchemaVersion: CurrentSchemaVersion,
		Batches:       make(map[string]*domain.Batch),
		Idempotency:   make(map[string]IdempotencyRecord),
	}
}

func loadSnapshot(path string) (Snapshot, error) {
	encoded, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return emptySnapshot(), nil
	}
	if err != nil {
		return Snapshot{}, fmt.Errorf("读取快照: %w", err)
	}
	var snapshot Snapshot
	if err := json.Unmarshal(encoded, &snapshot); err != nil {
		return Snapshot{}, fmt.Errorf("解析快照: %w", err)
	}
	if snapshot.SchemaVersion != CurrentSchemaVersion {
		return Snapshot{}, fmt.Errorf("%w: 得到 %d，需要 %d", ErrSchemaVersion, snapshot.SchemaVersion, CurrentSchemaVersion)
	}
	if snapshot.Batches == nil {
		snapshot.Batches = make(map[string]*domain.Batch)
	}
	if snapshot.Idempotency == nil {
		snapshot.Idempotency = make(map[string]IdempotencyRecord)
	}
	for id, batch := range snapshot.Batches {
		if id != batch.ID {
			return Snapshot{}, fmt.Errorf("快照索引 %s 与批次编号不一致", id)
		}
		if err := domain.ValidateRestoredBatch(batch); err != nil {
			return Snapshot{}, fmt.Errorf("校验批次 %s: %w", id, err)
		}
	}
	return snapshot, nil
}

func saveSnapshot(path string, snapshot Snapshot) error {
	snapshot.SchemaVersion = CurrentSchemaVersion
	snapshot.SavedAt = time.Now().UTC()
	encoded, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("编码快照: %w", err)
	}
	return writeFileAtomic(path, encoded, 0o600)
}
