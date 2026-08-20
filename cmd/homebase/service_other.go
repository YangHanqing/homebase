//go:build !darwin && !linux

package main

import "fmt"

func applyService(exe, configPath string) error {
	return fmt.Errorf("v1 hosts are macOS and Linux; this OS is not supported")
}

func stopService() error { return nil }

func serviceActive() bool { return false }

func logLocation() string { return "stderr" }

func logHint() string { return "this OS is not a supported Homebase host" }

func checkUserBus() error { return nil }

func warnSessionLinger() {}

func recoverAfterFailedStart(exe string) bool { return false }
