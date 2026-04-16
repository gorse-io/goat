package main

import "fmt"

func parseAssemblyForTarget(target, path string) (map[string][]Line, map[string]int, error) {
	switch target {
	case "amd64":
		return parseAssemblyAMD64(path)
	case "arm64":
		return parseAssemblyARM64(path)
	case "loong64":
		return parseAssemblyLoong64(path)
	case "riscv64":
		return parseAssemblyRISCV64(path)
	default:
		return nil, nil, fmt.Errorf("unsupported target architecture %q", target)
	}
}
