package main

import (
	"reflect"
	"testing"
)

func TestNormalizeArgs(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"无参数", nil, []string{}},
		{"全称原样透传", []string{"--config", "a.yaml"}, []string{"--config", "a.yaml"}},
		{"全称等号原样透传", []string{"--config=a.yaml"}, []string{"--config=a.yaml"}},
		{"简称换成全称", []string{"-c", "a.yaml"}, []string{"--config", "a.yaml"}},
		{"简称等号换成全称", []string{"-c=a.yaml"}, []string{"--config=a.yaml"}},
		{"无简称的参数用全称", []string{"--env-prefix", "acme"}, []string{"--env-prefix", "acme"}},
		{"布尔简称", []string{"-v"}, []string{"--version"}},
		{"帮助简称", []string{"-h"}, []string{"--help"}},
		{"位置参数原样", []string{"a.yaml"}, []string{"a.yaml"}},
		{"短横线原样", []string{"-"}, []string{"-"}},
		{"结束符之后不再改写", []string{"--", "-c"}, []string{"--", "-c"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeArgs(tt.in)
			if err != nil {
				t.Fatalf("normalizeArgs(%q) 报错: %v", tt.in, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("normalizeArgs(%q) = %q，期望 %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNormalizeArgsRejects(t *testing.T) {
	tests := []struct {
		name string
		in   []string
	}{
		{"单横线写全称", []string{"-config", "a.yaml"}},
		{"单横线写带值的全称", []string{"-env-prefix=acme"}},
		{"没有简称的参数不接受单字母", []string{"-e", "acme"}},
		{"未知简称", []string{"-x"}},
		{"不存在的长参数", []string{"-configx"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := normalizeArgs(tt.in); err == nil {
				t.Fatalf("normalizeArgs(%q) 应当报错，却返回 %q", tt.in, got)
			}
		})
	}
}

func TestParseArgs(t *testing.T) {
	opts, err := parseArgs([]string{"-c", "x.yaml", "--env-prefix", "acme", "-v"})
	if err != nil {
		t.Fatalf("parseArgs 报错: %v", err)
	}
	if opts.configPath != "x.yaml" {
		t.Fatalf("configPath = %q，期望 x.yaml", opts.configPath)
	}
	if opts.envPrefix != "acme" {
		t.Fatalf("envPrefix = %q，期望 acme", opts.envPrefix)
	}
	if !opts.version {
		t.Fatal("version 应当为 true")
	}
}

func TestParseArgsDefaults(t *testing.T) {
	opts, err := parseArgs(nil)
	if err != nil {
		t.Fatalf("parseArgs 报错: %v", err)
	}
	if opts.configPath != defaultConfigPath {
		t.Fatalf("configPath = %q，期望 %q", opts.configPath, defaultConfigPath)
	}
	if opts.version {
		t.Fatal("version 缺省应为 false")
	}
}
