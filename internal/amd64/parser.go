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

package amd64

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/gorse-io/goat/internal/common"
	"github.com/gorse-io/goat/internal/types"
)

var (
	amd64AttributeLine = regexp.MustCompile(`^\s+\..+$`)
	amd64NameLine      = regexp.MustCompile(`^\w+:.+$`)
	amd64LabelLine     = regexp.MustCompile(`^\.\w+_\d+:.*$`)
	amd64CodeLine      = regexp.MustCompile(`^\s+\w+.+$`)
)

// ParseAssembly parses AMD64 assembly file and returns functions map
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
		if amd64AttributeLine.MatchString(line) {
			continue
		} else if amd64NameLine.MatchString(line) {
			functionName = strings.Split(line, ":")[0]
			functions[functionName] = make([]types.Line, 0)
			labelName = ""
		} else if amd64LabelLine.MatchString(line) {
			labelName = strings.Split(line, ":")[0]
			labelName = labelName[1:]
			lines := functions[functionName]
			if len(lines) > 0 && lines[len(lines)-1].Assembly == "" {
				// If the last line is a label, append the label to the last line.
				lines[len(lines)-1].Labels = append(lines[len(lines)-1].Labels, labelName)
			} else {
				functions[functionName] = append(functions[functionName], types.Line{Labels: []string{labelName}})
			}
		} else if amd64CodeLine.MatchString(line) {
			asm := common.SanitizeAsm(line)
			if labelName == "" {
				functions[functionName] = append(functions[functionName], types.Line{Assembly: asm})
			} else {
				lines := functions[functionName]
				if len(lines) == 0 {
					functions[functionName] = append(functions[functionName], types.Line{Labels: []string{labelName}})
					lines = functions[functionName]
				}

				lines[len(lines)-1].Assembly = asm
				labelName = ""
			}
		}
	}

	if err = scanner.Err(); err != nil {
		return nil, nil, err
	}
	return functions, stackSizes, nil
}