// Copyright 2022 gorse Project Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by local law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package main

import "fmt"

func parseAssemblyForTarget(target, path string) (map[string][]Line, map[string]int, error) {
	switch target {
	case "amd64":
		return parseAssemblyX86(path)
	case "arm64":
		return parseAssemblyA64(path)
	case "loong64":
		return parseAssemblyLoong(path)
	case "riscv64":
		return parseAssemblyRv64(path)
	default:
		return nil, nil, fmt.Errorf("unsupported target architecture %q", target)
	}
}

func (t *TranslateUnit) generateGoAssemblyForTarget(target, path string, functions []Function) error {
	switch target {
	case "amd64":
		return t.generateGoAssemblyX86(path, functions)
	case "arm64":
		return t.generateGoAssemblyA64(path, functions)
	case "loong64":
		return t.generateGoAssemblyLoong(path, functions)
	case "riscv64":
		return t.generateGoAssemblyRv64(path, functions)
	default:
		return fmt.Errorf("unsupported target architecture %q", target)
	}
}
