package audit_cache_buffer_alias_test

import (
	"encoding/json"
	"testing"

	"pdconsole/internal/application"
	"pdconsole/internal/persistence"
)

func TestAuditCacheDoesNotLeakMutableDetails(t *testing.T) {
	store, err := persistence.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := application.NewService(store)
	batch, err := service.CreateBatch(application.CreateBatchCommand{
		IdempotencyKey: "create-audit-owner-boundary",
		CableSection:   "北站一段",
		CircuitName:    "甲回路",
		TestOwner:      "张工",
	})
	if err != nil {
		t.Fatal(err)
	}

	first, err := service.GetAuditTrail(batch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || len(first[0].Details) == 0 || !json.Valid(first[0].Details) {
		t.Fatalf("首次审计响应无效: %#v", first)
	}
	first[0].Details[0] = '['

	second, err := service.GetAuditTrail(batch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 1 || !json.Valid(second[0].Details) {
		t.Fatalf("再次读取的审计详情受到前一次响应污染: %q", second[0].Details)
	}
}
