package main

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"io"
	"os"
	"testing"
)

func Test_validateIpOrHostname(t *testing.T) {
	type args struct {
		input string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{"test", args{"1.2.3.4"}, "1.2.3.4", false},
		{"test", args{"1.2.3"}, "", true},
		{"test", args{"localhost"}, "localhost", false},
		{"test", args{"foo.bar"}, "foo.bar", false},
		{"test", args{"factorio.foo.bar"}, "factorio.foo.bar", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateIpOrHostname(tt.args.input)
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
	log = logrus.New()
	log.Out = io.Discard
	type args struct {
		key        string
		valueToSet interface{}
		defaultVal bool
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{"true string", args{"key", "true", true}, true},
		{"true bool", args{"key", true, true}, true},
		{"true int", args{"key", 1, true}, true},
		{"true empty", args{"key", nil, true}, true},
		{"true parse error default", args{"key", "xxx", true}, true},
		{"false string", args{"key", "false", true}, false},
		{"false bool", args{"key", false, true}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = os.Setenv(tt.args.key, fmt.Sprintf("%v", tt.args.valueToSet))
			if got := getEnvOrDefaultBool(tt.args.key, tt.args.defaultVal); got != tt.want {
				t.Errorf("getEnvOrDefaultBool() = %v, want %v", got, tt.want)
			}
		})
	}
}
