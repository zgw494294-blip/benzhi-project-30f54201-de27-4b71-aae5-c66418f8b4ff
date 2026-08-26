package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"isolation-chamber-commissioning/internal/application"
	"isolation-chamber-commissioning/internal/domain"
	"isolation-chamber-commissioning/internal/persistence"
	"isolation-chamber-commissioning/internal/verification"
	webadapter "isolation-chamber-commissioning/internal/web"
)

type config struct {
	addr      string
	dataDir   string
	selfcheck bool
}

func main() {
	cfg, err := parseConfig()
	if err != nil {
		log.Printf("配置错误：%v", err)
		os.Exit(2)
	}
	if err := run(cfg); err != nil {
		log.Printf("服务失败：%v", err)
		os.Exit(1)
	}
}

func parseConfig() (config, error) {
	addr := flag.String("addr", "", "监听地址，必须为回环地址")
	dataDir := flag.String("data", "data", "JSON 快照和审计日志目录")
	selfcheck := flag.Bool("selfcheck", false, "通过真实回环 HTTP 执行有界全流程自检")
	flag.Parse()
	resolved := strings.TrimSpace(*addr)
	if resolved == "" {
		if port := strings.TrimSpace(os.Getenv("PORT")); port != "" {
			value, err := strconv.Atoi(port)
			if err != nil || value < 1 || value > 65535 {
				return config{}, fmt.Errorf("PORT 必须是 1 到 65535 的端口号")
			}
			resolved = net.JoinHostPort("127.0.0.1", port)
		} else {
			resolved = "127.0.0.1:19081"
		}
	}
	if err := validateAddress(resolved); err != nil {
		return config{}, err
	}
	return config{addr: resolved, dataDir: *dataDir, selfcheck: *selfcheck}, nil
}

func validateAddress(addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("-addr 必须为 host:port：%w", err)
	}
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return fmt.Errorf("仅允许监听回环地址，拒绝 %q", host)
	}
	value, err := strconv.Atoi(port)
	if err != nil || value < 1 || value > 65535 {
		return fmt.Errorf("监听端口无效：%q", port)
	}
	return nil
}

func run(cfg config) error {
	dataDir := cfg.dataDir
	if cfg.selfcheck {
		temp, err := os.MkdirTemp("", "isolation-chamber-selfcheck-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(temp)
		dataDir = filepath.Join(temp, "store")
	}
	store, err := persistence.Open(dataDir)
	if err != nil {
		return fmt.Errorf("恢复本地案卷: %w", err)
	}
	defer store.Close()
	service := application.New(store, verification.New())
	server := &http.Server{Handler: webadapter.NewHandler(service), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second}
	listener, err := net.Listen("tcp", cfg.addr)
	if err != nil {
		return fmt.Errorf("监听 %s: %w", cfg.addr, err)
	}
	serverErr := make(chan error, 1)
	go func() {
		err := server.Serve(listener)
		if !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()
	if cfg.selfcheck {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		baseURL := "http://" + listener.Addr().String()
		checkErr := runSelfcheck(ctx, baseURL)
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer shutdownCancel()
		shutdownErr := server.Shutdown(shutdownCtx)
		if checkErr != nil {
			return checkErr
		}
		if shutdownErr != nil {
			return shutdownErr
		}
		fmt.Println("自检通过：已通过真实 HTTP 完成建案、冻结、四项测试、复核签发和凭据复算校验")
		return nil
	}
	log.Printf("隔离舱启用验证工作台监听 http://%s", listener.Addr().String())
	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case <-signalCtx.Done():
	case err := <-serverErr:
		if err != nil {
			return err
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return server.Shutdown(shutdownCtx)
}

type selfcheckClient struct {
	base     string
	client   *http.Client
	sequence int
}

func (c *selfcheckClient) request(ctx context.Context, method, path string, body any, target any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		c.sequence++
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", fmt.Sprintf("selfcheck-%02d", c.sequence))
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s 返回 %d：%s", method, path, resp.StatusCode, strings.TrimSpace(string(payload)))
	}
	if target != nil {
		if err := json.Unmarshal(payload, target); err != nil {
			return fmt.Errorf("解析 %s 响应: %w", path, err)
		}
	}
	return nil
}

func runSelfcheck(ctx context.Context, baseURL string) error {
	c := &selfcheckClient{base: baseURL, client: &http.Client{Timeout: 5 * time.Second}}
	if err := c.request(ctx, http.MethodGet, "/healthz", nil, nil); err != nil {
		return fmt.Errorf("健康检查: %w", err)
	}
	create := application.CreateCaseCommand{ChamberName: "自检隔离舱", Zones: []domain.ZoneBoundary{{Chamber: "自检隔离舱", Adjacent: "洁净走廊"}}, AirflowDirection: "洁净走廊 → 自检隔离舱", AcceptanceLimits: domain.AcceptanceLimits{PressureMinPa: 15, PressureDurationSec: 60, MaxLeakagePercent: 5, InterlockResponseSec: 2, RecoveryMaxMinutes: 20, RecoveryTargetParticles: 3520}, Actor: "自检验证工程师"}
	var commissioning domain.CommissioningCase
	if err := c.request(ctx, http.MethodPost, "/api/cases", create, &commissioning); err != nil {
		return fmt.Errorf("创建验证案: %w", err)
	}
	var preflight struct {
		Report domain.FreezeConfirmation `json:"report"`
	}
	if err := c.request(ctx, http.MethodPost, "/api/cases/"+commissioning.ID+"/freeze/preflight", application.FreezePreflightCommand{ExpectedVersion: commissioning.Version, Actor: "自检验证工程师"}, &preflight); err != nil {
		return fmt.Errorf("冻结预检: %w", err)
	}
	if err := c.request(ctx, http.MethodPost, "/api/cases/"+commissioning.ID+"/freeze", application.FreezeProtocolCommand{ExpectedVersion: commissioning.Version, FrozenBy: "自检验证工程师", ConfirmationToken: preflight.Report.Token}, &commissioning); err != nil {
		return fmt.Errorf("冻结方案: %w", err)
	}
	boolFalse := false
	measurements := map[string][]domain.Measurement{
		"pressure":     {{Name: "pressurePa", Value: 18, Unit: "Pa", OffsetSec: 0}, {Name: "pressurePa", Value: 17, Unit: "Pa", OffsetSec: 60}},
		"airtightness": {{Name: "leakagePercent", Value: 3, Unit: "%", OffsetSec: 0}},
		"interlock":    {{Name: "responseSec", Value: 1.2, Unit: "s", OffsetSec: 0}, {Name: "bothDoorsOpen", Unit: "boolean", OffsetSec: 0, Flag: &boolFalse}},
		"recovery":     {{Name: "recoveryMinutes", Value: 12, Unit: "min", OffsetSec: 0}, {Name: "particleCount", Value: 2800, Unit: "particles/m3", OffsetSec: 0}},
	}
	calibrationFrom := time.Now().UTC().Add(-24 * time.Hour)
	calibrationUntil := calibrationFrom.Add(48 * time.Hour)
	for _, checkpoint := range domain.DefaultCheckpoints() {
		now := time.Now().UTC()
		command := application.RecordRunCommand{ExpectedVersion: commissioning.Version, ProtocolRevision: commissioning.Protocol.Revision, CheckpointID: checkpoint.ID, Measurements: measurements[checkpoint.ID], InstrumentID: "INST-SELFCHECK", CertificateNumber: "CAL-SELFCHECK-2026", CalibrationValidFrom: calibrationFrom, CalibrationValidUntil: calibrationUntil, ApplicableKinds: domain.RequiredKinds, Witness: "自检见证人", StartedAt: now.Add(-time.Minute), CompletedAt: now, Actor: "自检见证人"}
		var response struct {
			Case *domain.CommissioningCase `json:"case"`
		}
		if err := c.request(ctx, http.MethodPost, "/api/cases/"+commissioning.ID+"/runs", command, &response); err != nil {
			return fmt.Errorf("执行检查点 %s: %w", checkpoint.ID, err)
		}
		commissioning = *response.Case
	}
	if commissioning.Status != domain.StatusReady {
		return fmt.Errorf("四项测试后状态为 %s", commissioning.Status)
	}
	if err := c.request(ctx, http.MethodPost, "/api/cases/"+commissioning.ID+"/submit-review", application.SubmitReviewCommand{ExpectedVersion: commissioning.Version, Actor: "自检验证工程师"}, &commissioning); err != nil {
		return fmt.Errorf("提交复核: %w", err)
	}
	answers := make([]domain.ChecklistAnswer, 0, len(commissioning.ReviewRounds[len(commissioning.ReviewRounds)-1].Items))
	for _, item := range commissioning.ReviewRounds[len(commissioning.ReviewRounds)-1].Items {
		answers = append(answers, domain.ChecklistAnswer{ItemID: item.ID, Confirmed: true})
	}
	if err := c.request(ctx, http.MethodPost, "/api/cases/"+commissioning.ID+"/review", application.ReviewCommand{ExpectedVersion: commissioning.Version, Decision: "APPROVE", Reviewer: "自检安全复核员", Answers: answers}, &commissioning); err != nil {
		return fmt.Errorf("批准案卷: %w", err)
	}
	if commissioning.Credential == nil {
		return errors.New("批准响应缺少启用凭据")
	}
	var verified application.CredentialVerification
	if err := c.request(ctx, http.MethodGet, "/api/credentials/"+commissioning.Credential.ID+"/verify", nil, &verified); err != nil {
		return fmt.Errorf("校验凭据: %w", err)
	}
	if !verified.Authentic {
		return errors.New("凭据复算结果不真实")
	}
	return nil
}
