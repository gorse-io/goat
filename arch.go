package main

import (
	"fmt"
	"runtime"
	"strings"
)

type archConfig struct {
	GoArch      string
	BuildTags   string
	BuildTarget string
}

var supportedArchConfigs = map[string]archConfig{
	"amd64": {
		GoArch:      "amd64",
		BuildTags:   "//go:build !noasm && amd64\n",
		BuildTarget: "amd64-linux-gnu",
	},
	"arm64": {
		GoArch:      "arm64",
		BuildTags:   "//go:build !noasm && arm64\n",
		BuildTarget: "arm64-linux-gnu",
	},
	"riscv64": {
		GoArch:      "riscv64",
		BuildTags:   "//go:build !noasm && riscv64\n",
		BuildTarget: "riscv64-linux-gnu",
	},
	"loong64": {
		GoArch:      "loong64",
		BuildTags:   "//go:build !noasm && loong64\n",
		BuildTarget: "loongarch64-linux-gnu",
	},
}

var targetArch = runtime.GOARCH

func selectedArch() (archConfig, error) {
	arch := strings.TrimSpace(targetArch)
	if arch == "" {
		arch = runtime.GOARCH
	}
	cfg, ok := supportedArchConfigs[arch]
	if !ok {
		return archConfig{}, fmt.Errorf("unsupported target architecture %q (supported: amd64, arm64, loong64, riscv64)", arch)
	}
	return cfg, nil
}
