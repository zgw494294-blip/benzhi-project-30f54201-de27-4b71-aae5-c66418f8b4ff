package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"pdconsole/internal/application"
	"pdconsole/internal/persistence"
	"pdconsole/internal/web"
)

func main() {
	configuration, err := parseConfig(os.Args[1:], os.Getenv("PORT"))
	if err != nil {
		log.Fatalf("配置错误: %v", err)
	}
	if configuration.SelfCheck {
		if err := runSelfCheck(configuration.Address); err != nil {
			log.Fatalf("selfcheck 失败: %v", err)
		}
		log.Printf("selfcheck 通过：HTTP 全流程、证书封存和重启恢复均正常")
		return
	}
	if err := runServer(configuration); err != nil {
		log.Fatalf("服务退出: %v", err)
	}
}

func runServer(configuration config) error {
	store, err := persistence.Open(configuration.DataDir)
	if err != nil {
		return fmt.Errorf("加载本地证据库: %w", err)
	}
	service := application.NewService(store)
	handler := web.NewServer(service).Handler()
	server := &http.Server{
		Addr: configuration.Address, Handler: handler,
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second,
		WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second,
	}
	shutdownSignal, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	serverError := make(chan error, 1)
	go func() {
		log.Printf("电缆局放诊断放行台已启动：http://%s", configuration.Address)
		serverError <- server.ListenAndServe()
	}()
	select {
	case err := <-serverError:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-shutdownSignal.Done():
		context, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(context); err != nil {
			return fmt.Errorf("优雅关闭 HTTP 服务: %w", err)
		}
		err := <-serverError
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}
