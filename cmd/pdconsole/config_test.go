package main

import "testing"

func TestConfigUsesLoopbackPortEnvironment(t *testing.T) {
	configuration, err := parseConfig(nil, "19123")
	if err != nil {
		t.Fatal(err)
	}
	if configuration.Address != "127.0.0.1:19123" {
		t.Fatalf("地址不正确: %s", configuration.Address)
	}
}

func TestConfigRejectsNonLoopback(t *testing.T) {
	if _, err := parseConfig([]string{"-addr=0.0.0.0:19081"}, ""); err == nil {
		t.Fatal("应拒绝 0.0.0.0")
	}
}
