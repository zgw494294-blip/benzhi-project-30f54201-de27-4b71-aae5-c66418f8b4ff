package domain

import (
	"errors"
	"testing"
	"time"
)

func validCase(t *testing.T) *CommissioningCase {
	t.Helper()
	now := time.Date(2026, 8, 26, 9, 0, 0, 0, time.UTC)
	c, err := CreateCase(NewCase{ID: "case-test", CaseNumber: "ICC-20260826-0001", ChamberName: "隔离舱 A", Zones: []ZoneBoundary{{Chamber: "隔离舱 A", Adjacent: "洁净走廊"}}, AirflowDirection: "洁净走廊 → 隔离舱 A", Limits: AcceptanceLimits{PressureMinPa: 15, PressureDurationSec: 60, MaxLeakagePercent: 5, InterlockResponseSec: 2, RecoveryMaxMinutes: 20, RecoveryTargetParticles: 3520}, Actor: "工程师", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestFreezeProtocolRejectsStaleVersionAndBecomesImmutableDuringExecution(t *testing.T) {
	c := validCase(t)
	now := c.CreatedAt.Add(time.Minute)
	if err := c.FreezeProtocol(0, "工程师", now, DefaultCheckpoints(), "rules/1"); err == nil {
		t.Fatal("应拒绝陈旧版本")
	} else {
		var de *DomainError
		if !errors.As(err, &de) || de.Code != CodeConflict {
			t.Fatalf("错误类型=%v", err)
		}
	}
	if err := c.FreezeProtocol(1, "工程师", now, DefaultCheckpoints(), "rules/1"); err != nil {
		t.Fatal(err)
	}
	if err := c.FreezeProtocol(c.Version, "工程师", now, DefaultCheckpoints(), "rules/1"); err == nil {
		t.Fatal("执行期不应允许覆盖冻结方案")
	}
}

func TestRunOrderingDeviationAndTargetedRetest(t *testing.T) {
	c := validCase(t)
	now := c.CreatedAt.Add(time.Minute)
	if err := c.FreezeProtocol(c.Version, "工程师", now, DefaultCheckpoints(), "rules/1"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ValidateRunTarget(c.Version, 1, "airtightness"); err == nil {
		t.Fatal("应阻止跳项")
	}
	attempt, err := c.ValidateRunTarget(c.Version, 1, "pressure")
	if err != nil || attempt != 1 {
		t.Fatalf("首次执行=%d %v", attempt, err)
	}
	fail := TestRun{ID: "run-1", CaseID: c.ID, ProtocolRevision: 1, CheckpointID: "pressure", Attempt: 1, Verdict: VerdictFail, FailureReasons: []string{"压差不足"}}
	if err := c.AddRun(c.Version, fail, "见证人", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if len(c.Deviations) != 1 || c.Status != StatusDeviation {
		t.Fatalf("未建立偏差：%+v", c)
	}
	if _, err := c.ValidateRunTarget(c.Version, 1, "pressure"); err == nil {
		t.Fatal("未整改前不得复测")
	}
	d := c.Deviations[0]
	if err := c.RemediateDeviation(c.Version, d.ID, "密封条错位", "更换并确认压紧", "工程师", now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	attempt, err = c.ValidateRunTarget(c.Version, 1, "pressure")
	if err != nil || attempt != 2 {
		t.Fatalf("定向复测=%d %v", attempt, err)
	}
	pass := fail
	pass.ID = "run-2"
	pass.Attempt = 2
	pass.Verdict = VerdictPass
	pass.FailureReasons = nil
	if err := c.AddRun(c.Version, pass, "见证人", now.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if c.Deviations[0].Status != DeviationClosed {
		t.Fatal("合格复测应关闭偏差")
	}
}

func TestReviewNeedsAllChecksAndReturnNeedsReason(t *testing.T) {
	c := validCase(t)
	now := c.CreatedAt.Add(time.Minute)
	if err := c.FreezeProtocol(c.Version, "工程师", now, DefaultCheckpoints(), "rules/1"); err != nil {
		t.Fatal(err)
	}
	if err := c.SubmitReview(c.Version, "工程师", now); err == nil {
		t.Fatal("未测试案卷不应送审")
	}
	c.Status = StatusReview
	if err := c.ReturnReview(c.Version, "复核员", "", now); err == nil {
		t.Fatal("无理由不得退回")
	}
}
