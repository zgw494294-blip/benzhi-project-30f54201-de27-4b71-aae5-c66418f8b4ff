package persistence

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"pdconsole/internal/domain"
)

type Store struct {
	mu           sync.RWMutex
	directory    string
	snapshotPath string
	auditPath    string
	snapshot     Snapshot
	clock        func() time.Time
}

func Open(directory string) (*Store, error) {
	directory = strings.TrimSpace(directory)
	if directory == "" {
		return nil, fmt.Errorf("数据目录不能为空")
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("创建数据目录: %w", err)
	}
	store := &Store{
		directory: directory, snapshotPath: filepath.Join(directory, "snapshot.json"),
		auditPath: filepath.Join(directory, "audit.log"), clock: time.Now,
	}
	snapshot, err := loadSnapshot(store.snapshotPath)
	if err != nil {
		return nil, err
	}
	events, err := readAndValidateAudit(store.auditPath)
	if err != nil {
		return nil, err
	}
	if len(events) > 0 {
		last := events[len(events)-1]
		if snapshot.Sequence != last.Sequence || snapshot.LastAuditHash != last.Hash {
			return nil, fmt.Errorf("%w: 快照与审计日志末端不一致", ErrAuditCorrupt)
		}
	} else if snapshot.Sequence != 0 || snapshot.LastAuditHash != "" {
		return nil, fmt.Errorf("%w: 快照声明了不存在的审计事件", ErrAuditCorrupt)
	}
	store.snapshot = snapshot
	return store, nil
}

func (s *Store) GetBatch(id string) (*domain.Batch, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	batch, ok := s.snapshot.Batches[id]
	if !ok {
		return nil, fmt.Errorf("%w: 批次 %s", ErrNotFound, id)
	}
	return domain.CloneBatch(batch)
}

func (s *Store) ListBatches() ([]*domain.Batch, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*domain.Batch, 0, len(s.snapshot.Batches))
	for _, batch := range s.snapshot.Batches {
		clone, err := domain.CloneBatch(batch)
		if err != nil {
			return nil, err
		}
		result = append(result, clone)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].UpdatedAt.After(result[j].UpdatedAt) })
	return result, nil
}

func (s *Store) GetIdempotency(key string, target any) (bool, error) {
	if strings.TrimSpace(key) == "" {
		return false, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.snapshot.Idempotency[key]
	if !ok {
		return false, nil
	}
	if err := json.Unmarshal(record.Response, target); err != nil {
		return false, fmt.Errorf("解析幂等结果: %w", err)
	}
	return true, nil
}

func (s *Store) Commit(change Commit) error {
	if change.Batch == nil || change.Batch.ID == "" {
		return fmt.Errorf("提交缺少批次")
	}
	if strings.TrimSpace(change.Operation) == "" || strings.TrimSpace(change.Actor) == "" {
		return fmt.Errorf("提交缺少操作名或操作人")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.snapshot.Idempotency[change.IdempotencyKey]; change.IdempotencyKey != "" && exists {
		return nil
	}
	if existing, exists := s.snapshot.Batches[change.Batch.ID]; exists && change.Operation == "batch.created" && existing != change.Batch {
		return fmt.Errorf("批次编号 %s 已存在", change.Batch.ID)
	}
	clone, err := domain.CloneBatch(change.Batch)
	if err != nil {
		return err
	}
	details, err := json.Marshal(change.Details)
	if err != nil {
		return fmt.Errorf("编码审计详情: %w", err)
	}
	response, err := json.Marshal(change.Response)
	if err != nil {
		return fmt.Errorf("编码幂等结果: %w", err)
	}
	next := cloneSnapshot(s.snapshot)
	next.Batches[clone.ID] = clone
	next.Sequence++
	event := AuditEvent{
		Sequence: next.Sequence, OccurredAt: s.clock().UTC(), Operation: change.Operation,
		BatchID: clone.ID, Actor: change.Actor, Version: clone.Version,
		Details: details, PreviousHash: next.LastAuditHash,
	}
	event.Hash, err = signAudit(event)
	if err != nil {
		return err
	}
	next.LastAuditHash = event.Hash
	if change.IdempotencyKey != "" {
		next.Idempotency[change.IdempotencyKey] = IdempotencyRecord{
			Key: change.IdempotencyKey, Operation: change.Operation, BatchID: clone.ID,
			Response: response, CreatedAt: s.clock().UTC(),
		}
	}
	// 先写审计日志，再替换快照；启动校验会拒绝任何半完成提交，避免静默接受不完整证据链。
	if err := appendAudit(s.auditPath, event); err != nil {
		return err
	}
	if err := saveSnapshot(s.snapshotPath, next); err != nil {
		return fmt.Errorf("审计已追加但快照保存失败，需要人工恢复: %w", err)
	}
	s.snapshot = next
	return nil
}

func cloneSnapshot(source Snapshot) Snapshot {
	next := source
	next.Batches = make(map[string]*domain.Batch, len(source.Batches)+1)
	for key, value := range source.Batches {
		next.Batches[key] = value
	}
	next.Idempotency = make(map[string]IdempotencyRecord, len(source.Idempotency)+1)
	for key, value := range source.Idempotency {
		next.Idempotency[key] = value
	}
	return next
}

func (s *Store) AuditEvents(batchID string) ([]AuditEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	events, err := readAndValidateAudit(s.auditPath)
	if err != nil {
		return nil, err
	}
	if batchID == "" {
		return events, nil
	}
	filtered := make([]AuditEvent, 0)
	for _, event := range events {
		if event.BatchID == batchID {
			filtered = append(filtered, event)
		}
	}
	return filtered, nil
}

func (s *Store) Health() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, err := readAndValidateAudit(s.auditPath)
	return err
}
