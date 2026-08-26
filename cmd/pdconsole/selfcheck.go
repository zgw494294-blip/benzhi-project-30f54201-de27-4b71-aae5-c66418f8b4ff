package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"pdconsole/internal/application"
	"pdconsole/internal/domain"
	"pdconsole/internal/persistence"
	"pdconsole/internal/web"
)

type selfCheckClient struct {
	baseURL string
	client  *http.Client
}

func runSelfCheck(address string) error {
	directory, err := os.MkdirTemp("", "pdconsole-selfcheck-*")
	if err != nil {
		return fmt.Errorf("创建自检目录: %w", err)
	}
	defer os.RemoveAll(directory)
	store, err := persistence.Open(directory)
	if err != nil {
		return err
	}
	service := application.NewService(store)
	server := &http.Server{
		Handler: web.NewServer(service).Handler(), ReadHeaderTimeout: 3 * time.Second,
		ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second,
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("监听自检地址 %s: %w", address, err)
	}
	serverDone := make(chan error, 1)
	go func() { serverDone <- server.Serve(listener) }()
	client := selfCheckClient{
		baseURL: "http://" + listener.Addr().String(),
		client:  &http.Client{Timeout: 5 * time.Second},
	}
	flowErr := client.runFlow()
	shutdownContext, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	shutdownErr := server.Shutdown(shutdownContext)
	<-serverDone
	if flowErr != nil {
		return flowErr
	}
	if shutdownErr != nil {
		return fmt.Errorf("关闭自检服务: %w", shutdownErr)
	}
	reopened, err := persistence.Open(directory)
	if err != nil {
		return fmt.Errorf("重启恢复校验失败: %w", err)
	}
	batches, err := reopened.ListBatches()
	if err != nil || len(batches) != 1 || batches[0].Status != domain.StatusSealed {
		return fmt.Errorf("重启后未恢复封存批次")
	}
	if _, err := os.Stat(filepath.Join(directory, "snapshot.json")); err != nil {
		return fmt.Errorf("未生成持久化快照: %w", err)
	}
	return nil
}

func (c selfCheckClient) runFlow() error {
	var health application.HealthStatus
	if err := c.get("/api/health", &health); err != nil || health.Status != "ok" {
		return fmt.Errorf("健康检查失败: %w", err)
	}
	created := domain.Batch{}
	if err := c.post("/api/batches", map[string]any{
		"idempotencyKey": "selfcheck-create", "cableSection": "自检电缆 A 段",
		"circuitName": "自检一回", "testOwner": "自检负责人",
	}, &created); err != nil {
		return fmt.Errorf("建批失败: %w", err)
	}
	points := []map[string]any{{
		"id": "P1", "name": "自检终端", "location": "A 相终端",
		"sensorRangePC": 100, "amplitudeLimitPC": 20,
		"trendLimitPercent": 25, "repeatabilityCount": 3,
	}}
	var scopePreflight domain.ScopePreflight
	if err := c.postBatch(created.ID, "/freeze/preflight", map[string]any{"points": points}, &scopePreflight); err != nil {
		return fmt.Errorf("冻结预检失败: %w", err)
	}
	if err := c.postBatch(created.ID, "/freeze", map[string]any{
		"idempotencyKey": "selfcheck-freeze", "expectedVersion": created.Version, "actor": "自检负责人",
		"points": points, "preflightScopeDigest": scopePreflight.ScopeDigest, "confirmed": true,
	}, &created); err != nil {
		return fmt.Errorf("冻结失败: %w", err)
	}
	if err := c.postBatch(created.ID, "/measurements", map[string]any{
		"idempotencyKey": "selfcheck-measurement", "expectedVersion": created.Version, "actor": "自检操作员",
		"pointID": "P1", "round": 1, "measuredAt": time.Now().UTC().Add(time.Second), "peakAmplitudePC": 8.5,
		"phaseSummary": "分布均匀且无异常特征", "temperatureC": 24.5, "humidityPercent": 52.0,
		"sensorSerial": "SELF-SENSOR-01", "operator": "自检操作员", "purpose": "initial",
	}, &created); err != nil {
		return fmt.Errorf("采集失败: %w", err)
	}
	if err := c.postBatch(created.ID, "/diagnose", map[string]any{
		"idempotencyKey": "selfcheck-diagnose", "expectedVersion": created.Version, "actor": "自检诊断员",
	}, &created); err != nil {
		return fmt.Errorf("诊断失败: %w", err)
	}
	if created.Status != domain.StatusReviewing || len(created.Deviations) != 0 {
		return fmt.Errorf("正常读数未进入安全复核状态")
	}
	var detail application.BatchDetail
	if err := c.get("/api/batches/"+created.ID, &detail); err != nil {
		return fmt.Errorf("复核就绪清单查询失败: %w", err)
	}
	if !detail.ReviewReadiness.Ready || detail.ReviewReadiness.Snapshot == nil {
		return fmt.Errorf("复核就绪清单未通过")
	}
	for index, reviewer := range []string{"自检复核甲", "自检复核乙"} {
		if err := c.postBatch(created.ID, "/reviews", map[string]any{
			"idempotencyKey":  fmt.Sprintf("selfcheck-review-%d", index),
			"expectedVersion": created.Version, "actor": reviewer, "reviewer": reviewer,
			"role": "安全复核专家", "approved": true, "opinion": "证据完整，同意复归",
			"evidenceDigest":  detail.ReviewReadiness.Snapshot.Digest,
			"evidenceVersion": detail.ReviewReadiness.Snapshot.BatchVersion,
		}, &created); err != nil {
			return fmt.Errorf("第 %d 次复核失败: %w", index+1, err)
		}
	}
	if err := c.postBatch(created.ID, "/issue", map[string]any{
		"idempotencyKey": "selfcheck-issue", "expectedVersion": created.Version, "actor": "自检签发员",
	}, &created); err != nil {
		return fmt.Errorf("签发失败: %w", err)
	}
	if created.Certificate == nil || created.Status != domain.StatusSealed || !created.Certificate.Verify() {
		return fmt.Errorf("证书未正确封存或摘要校验失败")
	}
	var certificate domain.Certificate
	if err := c.get("/api/batches/"+created.ID+"/certificate", &certificate); err != nil {
		return fmt.Errorf("下载证书失败: %w", err)
	}
	if certificate.ID != created.Certificate.ID {
		return fmt.Errorf("下载证书与封存证书不一致")
	}
	var verification application.CertificateVerification
	if err := c.get("/api/batches/"+created.ID+"/verification-package/verify", &verification); err != nil {
		return fmt.Errorf("证书摘要复算失败: %w", err)
	}
	if !verification.Valid || verification.CertificateID != certificate.ID {
		return fmt.Errorf("证书摘要复算结果不一致")
	}
	var verificationPackage domain.VerificationPackage
	if err := c.get("/api/batches/"+created.ID+"/verification-package", &verificationPackage); err != nil || verificationPackage.ContentDigest == "" {
		return fmt.Errorf("核验包下载失败: %w", err)
	}
	var audit struct {
		Count int `json:"count"`
	}
	if err := c.get("/api/batches/"+created.ID+"/audit", &audit); err != nil {
		return fmt.Errorf("审计链查询失败: %w", err)
	}
	if audit.Count != 7 {
		return fmt.Errorf("审计事件数异常: 得到 %d，需要 7", audit.Count)
	}
	return nil
}

func (c selfCheckClient) postBatch(batchID, suffix string, body any, target any) error {
	return c.post("/api/batches/"+batchID+suffix, body, target)
}

func (c selfCheckClient) post(path string, body any, target any) error {
	encoded, err := json.Marshal(body)
	if err != nil {
		return err
	}
	request, err := http.NewRequest(http.MethodPost, c.baseURL+path, bytes.NewReader(encoded))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	return c.do(request, target)
}

func (c selfCheckClient) get(path string, target any) error {
	request, err := http.NewRequest(http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	return c.do(request, target)
}

func (c selfCheckClient) do(request *http.Request, target any) error {
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		content, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("HTTP %d: %s", response.StatusCode, string(content))
	}
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		return err
	}
	return nil
}
