package persistence

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"pdconsole/internal/domain"
)

func TestStorePersistsAndRestores(t *testing.T) {
	directory := t.TempDir()
	store, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	batch, _ := domain.NewBatch("b1", "区段", "回路", "负责人", time.Now().UTC())
	if err := store.Commit(Commit{Batch: batch, Operation: "batch.created", Actor: "负责人", Details: map[string]string{"test": "yes"}, IdempotencyKey: "create:k", Response: batch}); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := reopened.GetBatch("b1")
	if err != nil || restored.CableSection != "区段" {
		t.Fatalf("恢复失败: %#v, %v", restored, err)
	}
	var cached domain.Batch
	if found, err := reopened.GetIdempotency("create:k", &cached); err != nil || !found || cached.ID != "b1" {
		t.Fatalf("幂等结果恢复失败: %v %v %#v", found, err, cached)
	}
}

func TestStoreRejectsTamperedAudit(t *testing.T) {
	directory := t.TempDir()
	store, _ := Open(directory)
	batch, _ := domain.NewBatch("b1", "区段", "回路", "负责人", time.Now().UTC())
	if err := store.Commit(Commit{Batch: batch, Operation: "batch.created", Actor: "负责人", Details: nil, Response: batch}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "audit.log")
	content, _ := os.ReadFile(path)
	content[10] ^= 1
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(directory); err == nil {
		t.Fatal("篡改审计日志后应拒绝启动")
	}
}
