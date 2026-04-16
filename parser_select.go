// Copyright 2022 gorse Project Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package main

import (
	"fmt"

	"github.com/gorse-io/goat/internal/amd64"
	"github.com/gorse-io/goat/internal/arm64"
	"github.com/gorse-io/goat/internal/loong64"
	"github.com/gorse-io/goat/internal/riscv64"
	"github.com/gorse-io/goat/internal/types"
)

func parseAssemblyForTarget(target, path string) (map[string][]types.Line, map[string]int, error) {
	switch target {
	case "amd64":
		return amd64.ParseAssembly(path)
	case "arm64":
		return arm64.ParseAssembly(path)
	case "loong64":
		return loong64.ParseAssembly(path)
	case "riscv64":
		return riscv64.ParseAssembly(path)
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