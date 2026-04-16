//go:build !noasm

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

package riscv64

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/gorse-io/goat/internal/types"
)

var (
	riscv64AttributeLine = regexp.MustCompile(`^\s+\..+$`)
	riscv64NameLine      = regexp.MustCompile(`^\w+:.+$`)
	riscv64LabelLine     = regexp.MustCompile(`^\.\w+_\d+:.*$`)
	riscv64CodeLine      = regexp.MustCompile(`^\s+\w+.+$`)
)

// ParseAssembly parses RISCV64 assembly file and returns functions map
func ParseAssembly(path string) (map[string][]types.Line, map[string]int, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer func(file *os.File) {
		if err = file.Close(); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}(file)

	var (
		stackSizes   = make(map[string]int)
		functions    = make(map[string][]types.Line)
		functionName string
		labelName    string
	)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if riscv64AttributeLine.MatchString(line) {
			continue
		} else if riscv64NameLine.MatchString(line) {
			functionName = strings.Split(line, ":")[0]
			functions[functionName] = make([]types.Line, 0)
		} else if riscv64LabelLine.MatchString(line) {
			labelName = strings.Split(line, ":")[0]
			labelName = labelName[1:]
			lines := functions[functionName]
			if len(lines) == 1 || lines[len(lines)-1].Assembly != "" {
				functions[functionName] = append(functions[functionName], types.Line{Labels: []string{labelName}})
			} else {
				lines[len(lines)-1].Labels = append(lines[len(lines)-1].Labels, labelName)
			}
		} else if riscv64CodeLine.MatchString(line) {
			asm := strings.Split(line, "//")[0]
			asm = strings.TrimSpace(asm)
			if labelName == "" {
				functions[functionName] = append(functions[functionName], types.Line{Assembly: asm})
			} else {
				lines := functions[functionName]
				if len(lines) > 0 {
					lines[len(lines)-1].Assembly = asm
				}
				labelName = ""
			}
		}
	}

	if err = scanner.Err(); err != nil {
		return nil, nil, err
	}
	return functions, stackSizes, nil
}