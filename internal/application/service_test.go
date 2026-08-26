package application

import (
	"errors"
	"testing"

	"pdconsole/internal/domain"
	"pdconsole/internal/persistence"
)

func TestIdempotencyAndVersionConflict(t *testing.T) {
	store, err := persistence.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(store)
	command := CreateBatchCommand{IdempotencyKey: "create-1", CableSection: "区段", CircuitName: "回路", TestOwner: "负责人"}
	first, err := service.CreateBatch(command)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.CreateBatch(command)
	if err != nil || first.ID != second.ID {
		t.Fatalf("重复建批应返回同一结果: %v", err)
	}
	_, err = service.FreezeScope(first.ID, FreezeScopeCommand{
		CommandMeta: CommandMeta{IdempotencyKey: "freeze", ExpectedVersion: first.Version + 1, Actor: "负责人"},
		Points:      []domain.TestPoint{{ID: "P", Name: "点", Location: "位置", SensorRangePC: 100, AmplitudeLimitPC: 20, TrendLimitPercent: 20, RepeatabilityCount: 2}},
	})
	if !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("应返回版本冲突，得到 %v", err)
	}
}

func TestDuplicateConfirmationAndSourceReuse(t *testing.T) {
	store, err := persistence.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(store)
	source, err := service.CreateBatch(CreateBatchCommand{IdempotencyKey: "source", CableSection: "甲段", CircuitName: "一回", TestOwner: "张工"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.CreateBatch(CreateBatchCommand{IdempotencyKey: "duplicate", CableSection: "甲段", CircuitName: "一回", TestOwner: "李工"})
	var duplicate DuplicateBatchError
	if !errors.As(err, &duplicate) || len(duplicate.Matches) != 1 || duplicate.Matches[0].ID != source.ID {
		t.Fatalf("未确认重复建批应返回候选批次: %v", err)
	}
	reused, err := service.CreateBatch(CreateBatchCommand{IdempotencyKey: "reuse", SourceBatchID: source.ID, ConfirmDuplicate: true})
	if err != nil {
		t.Fatal(err)
	}
	again, err := service.CreateBatch(CreateBatchCommand{IdempotencyKey: "reuse", SourceBatchID: source.ID, ConfirmDuplicate: true})
	if err != nil || again.ID != reused.ID {
		t.Fatal("复用建批重复提交应返回同一新批次")
	}
	if reused.ID == source.ID || reused.SourceBatchID != source.ID || reused.CableSection != source.CableSection || reused.TestOwner != source.TestOwner {
		t.Fatal("复用批次基础资料或来源关系错误")
	}
	if reused.Status != domain.StatusDraft || len(reused.Measurements) != 0 || len(reused.Deviations) != 0 || len(reused.Reviews) != 0 || reused.Certificate != nil {
		t.Fatal("复用不得复制业务证据和状态")
	}
}
