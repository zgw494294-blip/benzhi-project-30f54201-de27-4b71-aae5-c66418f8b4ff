package main

import (
	"flag"
	"fmt"
	"net"
	"strconv"
	"strings"
)

const defaultAddress = "127.0.0.1:19081"

type config struct {
	Address   string
	DataDir   string
	SelfCheck bool
}

func parseConfig(arguments []string, portEnvironment string) (config, error) {
	set := flag.NewFlagSet("pdconsole", flag.ContinueOnError)
	address := set.String("addr", defaultAddress, "HTTP 监听地址（仅允许回环地址）")
	dataDir := set.String("data", "./data", "本地快照与审计日志目录")
	selfCheck := set.Bool("selfcheck", false, "运行有界 HTTP 全流程自检并退出")
	if err := set.Parse(arguments); err != nil {
		return config{}, err
	}
	if set.NArg() != 0 {
		return config{}, fmt.Errorf("不支持位置参数: %s", strings.Join(set.Args(), " "))
	}
	addressExplicit := false
	set.Visit(func(value *flag.Flag) {
		if value.Name == "addr" {
			addressExplicit = true
		}
	})
	resolved := strings.TrimSpace(*address)
	if !addressExplicit && strings.TrimSpace(portEnvironment) != "" {
		port, err := strconv.Atoi(strings.TrimSpace(portEnvironment))
		if err != nil || port < 1 || port > 65535 {
			return config{}, fmt.Errorf("PORT 必须是 1 到 65535 的端口号")
		}
		resolved = net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	}
	if err := validateAddress(resolved); err != nil {
		return config{}, err
	}
	if strings.TrimSpace(*dataDir) == "" {
		return config{}, fmt.Errorf("-data 不能为空")
	}
	return config{Address: resolved, DataDir: *dataDir, SelfCheck: *selfCheck}, nil
}

func validateAddress(address string) error {
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("-addr 必须使用 host:port 格式: %w", err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("-addr 仅允许使用数值回环地址，如 127.0.0.1:19081")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("-addr 端口必须在 1 到 65535 之间")
	}
	return nil
}
