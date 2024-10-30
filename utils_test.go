package main

import "testing"

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
