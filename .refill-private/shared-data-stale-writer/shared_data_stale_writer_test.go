package shared_data_stale_writer_test

import (
	"testing"
	"time"

	"pdconsole/internal/domain"
	"pdconsole/internal/persistence"
)

func TestSecondStoreCannotCommitFromStaleSnapshot(t *testing.T) {
	directory := t.TempDir()
	first, err := persistence.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	second, err := persistence.Open(directory)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 8, 27, 9, 0, 0, 0, time.UTC)
	firstBatch, _ := domain.NewBatch("first-batch", "甲段", "一回", "甲", now)
	secondBatch, _ := domain.NewBatch("second-batch", "乙段", "二回", "乙", now)

	firstDone := make(chan struct{})
	firstResult := make(chan error, 1)
	secondResult := make(chan error, 1)
	go func() {
		firstResult <- first.Commit(persistence.Commit{
			Batch: firstBatch, Operation: "batch.created", Actor: "甲", Response: firstBatch,
		})
		close(firstDone)
	}()
	go func() {
		<-firstDone
		secondResult <- second.Commit(persistence.Commit{
			Batch: secondBatch, Operation: "batch.created", Actor: "乙", Response: secondBatch,
		})
	}()

	if err := <-firstResult; err != nil {
		t.Fatalf("首个写入失败: %v", err)
	}
	if err := <-secondResult; err == nil {
		t.Fatal("第二个 Store 使用陈旧快照提交时应被拒绝")
	}
	if err := first.Health(); err != nil {
		t.Fatalf("拒绝陈旧写入后审计链应保持健康: %v", err)
	}
}
