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

package common

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/gorse-io/goat/internal/types"
)

// Shared regex patterns for objdump parsing
var (
	SymbolLine = regexp.MustCompile(`^\w+\s+<\w+>:$`)
	DataLine   = regexp.MustCompile(`^\w+:\s+\w+\s+.+$`)
)

// SanitizeAsm cleans up assembly instruction text
func SanitizeAsm(asm string) string {
	asm = strings.TrimSpace(asm)
	asm = strings.Split(asm, "//")[0]
	asm = strings.TrimSpace(asm)
	return asm
}

// ParseObjectDump parses objdump output and fills Binary field
func ParseObjectDump(dump string, functions map[string][]types.Line) error {
	var (
		functionName string
		lineNumber   int
	)
	for i, line := range strings.Split(dump, "\n") {
		line = strings.TrimSpace(line)
		if SymbolLine.MatchString(line) {
			functionName = strings.Split(line, "<")[1]
			functionName = strings.Split(functionName, ">")[0]
			lineNumber = 0
		} else if DataLine.MatchString(line) {
			data := strings.Split(line, ":")[1]
			data = strings.TrimSpace(data)
			splits := strings.Split(data, " ")
			var binary []string
			var assembly string
			in_asm := false
			for _, s := range splits {
				if in_asm {
					assembly = assembly + " " + s
				} else if s == "" {
					continue
				} else {
					// Check if all hex chars
					all_hex := true
					for _, c := range s {
						if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
							all_hex = false
							break
						}
					}
					if all_hex {
						for j := 0; j < len(s); j += 2 {
							binary = append(binary, s[j:j+2])
						}
					} else {
						assembly = s
						in_asm = true
					}
				}
			}
			assembly = SanitizeAsm(strings.TrimSpace(assembly))
			if strings.Contains(assembly, "nop") || assembly == "" ||
				strings.HasPrefix(assembly, "nop") || assembly == "xchg   %ax,%ax" ||
				assembly == "cs nopw 0x0(%rax,%rax,1)" {
				continue
			}
			if lineNumber >= len(functions[functionName]) {
				return fmt.Errorf("%d: unexpected objectdump line: %s", i, line)
			}
			functions[functionName][lineNumber].Binary = binary
			lineNumber++
		}
	}
	return nil
}