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
package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
	"unicode"

	"github.com/klauspost/asmfmt"
	"github.com/samber/lo"
)

const (
)

var (
	arm64AttributeLine = regexp.MustCompile(`^\s+\..+$`)
	arm64NameLine      = regexp.MustCompile(`^\w+:.+$`)
	arm64LabelLine     = regexp.MustCompile(`^\.\w+_\d+:.*$`)
	arm64CodeLine      = regexp.MustCompile(`^\s+\w+.+$`)
	arm64JmpLine       = regexp.MustCompile(`^(b|b\.\w{2})\t\.\w+_\d+$`)


	arm64Registers   = []string{"R0", "R1", "R2", "R3", "R4", "R5", "R6", "R7"}
	arm64FpRegisters = []string{"F0", "F1", "F2", "F3", "F4", "F5", "F6", "F7"}
)


func formatLineARM64(line *Line) string {
	var builder strings.Builder
	if arm64JmpLine.MatchString(line.Assembly) {
		splits := strings.Split(line.Assembly, "\t")
		instruction := strings.Map(func(r rune) rune {
			if r == '.' {
				return -1
			}
			return unicode.ToUpper(r)
		}, splits[0])
		label := splits[1][1:]
		builder.WriteString(fmt.Sprintf("%s %s\n", instruction, label))
	} else {
		builder.WriteString("\t")
		builder.WriteString(fmt.Sprintf("WORD $0x%v", line.Binary))
		builder.WriteString("\t// ")
		builder.WriteString(line.Assembly)
		builder.WriteString("\n")
	}
	return builder.String()
}

func parseAssemblyA64(path string) (map[string][]Line, map[string]int, error) {
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
		functions    = make(map[string][]Line)
		functionName string
		labelName    string
	)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if arm64AttributeLine.MatchString(line) {
			continue
		} else if arm64NameLine.MatchString(line) {
			functionName = strings.Split(line, ":")[0]
			functions[functionName] = make([]Line, 0)
		} else if arm64LabelLine.MatchString(line) {
			labelName = strings.Split(line, ":")[0]
			labelName = labelName[1:]
			lines := functions[functionName]
			if len(lines) == 1 || lines[len(lines)-1].Assembly != "" {
				functions[functionName] = append(functions[functionName], Line{Labels: []string{labelName}})
			} else {
				lines[len(lines)-1].Labels = append(lines[len(lines)-1].Labels, labelName)
			}
		} else if arm64CodeLine.MatchString(line) {
			asm := strings.Split(line, "//")[0]
			asm = strings.TrimSpace(asm)
			if labelName == "" {
				functions[functionName] = append(functions[functionName], Line{Assembly: asm})
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


func (t *TranslateUnit) generateGoAssemblyA64(path string, functions []Function) error {
	// generate code
	var builder strings.Builder
	builder.WriteString(t.Arch.BuildTags)
	t.writeHeader(&builder)
	for _, function := range functions {
		returnSize := 0
		if function.Type != "void" {
			returnSize += 8
		}
		registerCount, fpRegisterCount, offset := 0, 0, 0
		var stack []lo.Tuple2[int, Parameter]
		var argsBuilder strings.Builder
		for _, param := range function.Parameters {
			sz := 8
			if param.Pointer {
				sz = 8
			} else {
				sz = supportedTypes[param.Type]
			}
			if offset%sz != 0 {
				offset += sz - offset%sz
			}
			if !param.Pointer && (param.Type == "float" || param.Type == "double") {
				if fpRegisterCount < len(arm64FpRegisters) {
					if param.Type == "float" {
						argsBuilder.WriteString(fmt.Sprintf("\tFMOVS %s+%d(FP), %s\n", param.Name, offset, arm64FpRegisters[fpRegisterCount]))
					} else {
						argsBuilder.WriteString(fmt.Sprintf("\tFMOVD %s+%d(FP), %s\n", param.Name, offset, arm64FpRegisters[fpRegisterCount]))
					}
					fpRegisterCount++
				} else {
					stack = append(stack, lo.Tuple2[int, Parameter]{A: offset, B: param})
				}
			} else {
				if registerCount < len(arm64Registers) {
					argsBuilder.WriteString(fmt.Sprintf("\tMOVD %s+%d(FP), %s\n", param.Name, offset, arm64Registers[registerCount]))
					registerCount++
				} else {
					stack = append(stack, lo.Tuple2[int, Parameter]{A: offset, B: param})
				}
			}
			offset += sz
		}
		if offset%8 != 0 {
			offset += 8 - offset%8
		}
		stackOffset := 0
		if len(stack) > 0 {
			for i := 0; i < len(stack); i++ {
				argsBuilder.WriteString(fmt.Sprintf("\tMOVD %s+%d(FP), R8\n", stack[i].B.Name, stack[i].A))
				argsBuilder.WriteString(fmt.Sprintf("\tMOVD R8, %d(RSP)\n", stackOffset))
				if stack[i].B.Pointer {
					stackOffset += 8
				} else {
					stackOffset += supportedTypes[stack[i].B.Type]
				}
			}
		}
		if stackOffset%8 != 0 {
			stackOffset += 8 - stackOffset%8
		}
		builder.WriteString(fmt.Sprintf("\nTEXT ·%v(SB), $%d-%d\n",
			function.Name, stackOffset, offset+returnSize))
		builder.WriteString(argsBuilder.String())
		for _, line := range function.Lines {
			for _, label := range line.Labels {
				builder.WriteString(label)
				builder.WriteString(":\n")
			}
			if line.Assembly == "ret" {
				if function.Type != "void" {
					switch function.Type {
					case "int64_t", "long", "_Bool":
						builder.WriteString(fmt.Sprintf("\tMOVD R0, result+%d(FP)\n", offset))
					case "double":
						builder.WriteString(fmt.Sprintf("\tFMOVD F0, result+%d(FP)\n", offset))
					case "float":
						builder.WriteString(fmt.Sprintf("\tFMOVS F0, result+%d(FP)\n", offset))
					default:
						return fmt.Errorf("unsupported return type: %v", function.Type)
					}
				}
				builder.WriteString("\tRET\n")
			} else {
				builder.WriteString(formatLineARM64(&line))
			}
		}
	}

	// write file
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func(f *os.File) {
		if err = f.Close(); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}(f)
	bytes, err := asmfmt.Format(strings.NewReader(builder.String()))
	if err != nil {
		return err
	}
	_, err = f.Write(bytes)
	return err
}
