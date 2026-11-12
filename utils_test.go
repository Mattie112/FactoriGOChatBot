package main

import (
	"github.com/sirupsen/logrus"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func Test_validateIpOrHostname(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"1.2.3.4", "1.2.3.4", false},
		{"::1", "::1", false},
		{"1.2.3", "", true},
		{"localhost", "localhost", false},
		{"foo.bar", "foo.bar", false},
		{"factorio.foo.bar", "factorio.foo.bar", false},
		{"factorio-1", "factorio-1", false},
		{"1factorio.foo.bar", "1factorio.foo.bar", false},
		{"factorio_server", "factorio_server", false},
		{"", "", true},
		{"foo bar", "", true},
		{"foo..bar", "", true},
		{"-foo.bar", "", true},
		{"foo.bar:1234", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := validateIpOrHostname(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateIpOrHostname() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("validateIpOrHostname() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getEnvOrDefaultBool(t *testing.T) {
	const key = "FGCB_TEST_BOOL"
	tests := []struct {
		name       string
		set        bool
		value      string
		defaultVal bool
		want       bool
	}{
		{"true", true, "true", false, true},
		{"TRUE", true, "TRUE", false, true},
		{"1", true, "1", false, true},
		{"false", true, "false", true, false},
		{"0", true, "0", true, false},
		{"not set, default true", false, "", true, true},
		{"not set, default false", false, "", false, false},
		{"empty, default true", true, "", true, true},
		{"parse error, default true", true, "xxx", true, true},
		{"parse error, default false", true, "xxx", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// t.Setenv restores the original value after the test
			t.Setenv(key, tt.value)
			if !tt.set {
				_ = os.Unsetenv(key)
			}
			if got := getEnvOrDefaultBool(key, tt.defaultVal); got != tt.want {
				t.Errorf("getEnvOrDefaultBool() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getLoggerFromConfig(t *testing.T) {
	tests := []struct {
		logLevel string
		want     logrus.Level
	}{
		{"debug", logrus.DebugLevel},
		{"DEBUG", logrus.DebugLevel},
		{"info", logrus.InfoLevel},
		{"warning", logrus.WarnLevel},
		{"warn", logrus.WarnLevel},
		{"error", logrus.ErrorLevel},
		{"fatal", logrus.FatalLevel},
		{"", logrus.InfoLevel},
		{"foobar", logrus.InfoLevel},
	}
	for _, tt := range tests {
		t.Run(tt.logLevel, func(t *testing.T) {
			if got := getLoggerFromConfig(tt.logLevel).Level; got != tt.want {
				t.Errorf("getLoggerFromConfig() level = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_schedule(t *testing.T) {
	var calls atomic.Int32
	quit := schedule(10*time.Millisecond, func() {
		calls.Add(1)
	})

	// The delay used to be multiplied by a second (again), so the task never ran
	deadline := time.Now().Add(5 * time.Second)
	for calls.Load() < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("schedule() ran the task %d times, expected at least 2", calls.Load())
		}
		time.Sleep(5 * time.Millisecond)
	}

	close(quit)
	time.Sleep(50 * time.Millisecond) // Let a tick that is already in progress finish
	afterQuit := calls.Load()
	time.Sleep(50 * time.Millisecond)
	if got := calls.Load(); got != afterQuit {
		t.Errorf("schedule() still ran the task after quit (%d -> %d)", afterQuit, got)
	}
}

func Test_loadConfig(t *testing.T) {
	tests := []struct {
		name    string
		ip      string
		port    string
		wantErr bool
	}{
		{"valid", "factorio.local", "27015", false},
		{"invalid ip", "not valid!", "27015", true},
		{"port is not a number", "factorio.local", "abc", true},
		{"port out of range", "factorio.local", "70000", true},
		{"port zero", "factorio.local", "0", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("RCON_IP", tt.ip)
			t.Setenv("RCON_PORT", tt.port)
			got, err := loadConfig()
			if (err != nil) != tt.wantErr {
				t.Fatalf("loadConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && (got.rconIp != tt.ip || got.rconPort != 27015) {
				t.Errorf("loadConfig() = %s:%d, want %s:27015", got.rconIp, got.rconPort, tt.ip)
			}
		})
	}
}
