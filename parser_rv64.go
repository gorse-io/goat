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

	"github.com/gorse-io/goat/internal/types"
	"github.com/klauspost/asmfmt"
	"github.com/samber/lo"
)

const (
)

var (
	riscv64AttributeLine = regexp.MustCompile(`^\s+\..+$`)
	riscv64NameLine      = regexp.MustCompile(`^\w+:.+$`)
	riscv64LabelLine     = regexp.MustCompile(`^\.\w+_\d+:.*$`)
	riscv64CodeLine      = regexp.MustCompile(`^\s+\w+.+$`)


	riscv64Registers   = []string{"A0", "A1", "A2", "A3", "A4", "A5", "A6", "A7"}
	riscv64FpRegisters = []string{"FA0", "FA1", "FA2", "FA3", "FA4", "FA5", "FA6", "FA7"}
)


func formatLineRISCV64(line *Line) string {
	var builder strings.Builder
	builder.WriteString("\t")
	if strings.HasPrefix(line.Assembly, "b") {
		splits := strings.Split(line.Assembly, ".")
		op := strings.TrimSpace(splits[0])
		operand := splits[1]
		builder.WriteString(fmt.Sprintf("%s %s", strings.ToUpper(op), operand))
	} else if strings.HasPrefix(line.Assembly, "j") {
		splits := strings.Split(line.Assembly, "\t")
		label := splits[1][1:]
		builder.WriteString(fmt.Sprintf("JMP %s\n", label))
	} else {
		if len(line.Binary) == 8 {
			builder.WriteString(fmt.Sprintf("WORD $0x%v", line.Binary))
		} else {
			_, _ = fmt.Fprintln(os.Stderr, "compressed instructions are not supported.")
			os.Exit(1)
		}
		builder.WriteString("\t// ")
		builder.WriteString(line.Assembly)
	}
	builder.WriteString("\n")
	return builder.String()
}

func parseAssemblyRv64(path string) (map[string][]Line, map[string]int, error) {
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
		if riscv64AttributeLine.MatchString(line) {
			continue
		} else if riscv64NameLine.MatchString(line) {
			functionName = strings.Split(line, ":")[0]
			functions[functionName] = make([]Line, 0)
		} else if riscv64LabelLine.MatchString(line) {
			labelName = strings.Split(line, ":")[0]
			labelName = labelName[1:]
			lines := functions[functionName]
			if len(lines) == 1 || lines[len(lines)-1].Assembly != "" {
				functions[functionName] = append(functions[functionName], Line{Labels: []string{labelName}})
			} else {
				lines[len(lines)-1].Labels = append(lines[len(lines)-1].Labels, labelName)
			}
		} else if riscv64CodeLine.MatchString(line) {
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


func (t *TranslateUnit) generateGoAssemblyRv64(path string, functions []Function) error {
	// generate code
	var builder strings.Builder
	builder.WriteString(t.Arch.BuildTags)
	t.writeHeader(&builder)
	for _, function := range functions {
		returnSize := 0
		if function.Type != "void" {
			returnSize += 8
		}
		builder.WriteString(fmt.Sprintf("\nTEXT ·%v(SB), $%d-%d\n",
			function.Name, returnSize, len(function.Parameters)*8))
		registerCount, fpRegisterCount, offset := 0, 0, 0
		var stack []lo.Tuple2[int, Parameter]
		for _, param := range function.Parameters {
			sz := 8
			if param.Pointer {
				sz = 8
			} else {
				sz = types.SupportedTypes[param.Type]
			}
			if offset%sz != 0 {
				offset += sz - offset%sz
			}
			if !param.Pointer && (param.Type == "double" || param.Type == "float") {
				if fpRegisterCount < len(riscv64FpRegisters) {
					if param.Type == "double" {
						builder.WriteString(fmt.Sprintf("\tMOVD %s+%d(FP), %s\n", param.Name, offset, riscv64FpRegisters[fpRegisterCount]))
					} else {
						builder.WriteString(fmt.Sprintf("\tMOVF %s+%d(FP), %s\n", param.Name, offset, riscv64FpRegisters[fpRegisterCount]))
					}
					fpRegisterCount++
				} else {
					stack = append(stack, lo.Tuple2[int, Parameter]{A: offset, B: param})
				}
			} else {
				if registerCount < len(riscv64Registers) {
					if param.Type == "_Bool" {
						builder.WriteString(fmt.Sprintf("\tMOVB %s+%d(FP), %s\n", param.Name, offset, riscv64Registers[registerCount]))
					} else {
						builder.WriteString(fmt.Sprintf("\tMOV %s+%d(FP), %s\n", param.Name, offset, riscv64Registers[registerCount]))
					}
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
		frameSize := 0
		if len(stack) > 0 {
			for i := 0; i < len(stack); i++ {
				if stack[i].B.Pointer {
					frameSize += 8
				} else {
					frameSize += types.SupportedTypes[stack[i].B.Type]
				}
			}
			builder.WriteString(fmt.Sprintf("\tADDI -%d, SP, SP\n", frameSize))
			stackoffset := 0
			for i := 0; i < len(stack); i++ {
				builder.WriteString(fmt.Sprintf("\tMOV %s+%d(FP), T0\n", stack[i].B.Name, frameSize+stack[i].A))
				builder.WriteString(fmt.Sprintf("\tMOV T0, %d(SP)\n", stackoffset))
				if stack[i].B.Pointer {
					stackoffset += 8
				} else {
					stackoffset += types.SupportedTypes[stack[i].B.Type]
				}
			}
		}
		for _, line := range function.Lines {
			for _, label := range line.Labels {
				builder.WriteString(label)
				builder.WriteString(":\n")
			}
			if line.Assembly == "ret" {
				if frameSize > 0 {
					builder.WriteString(fmt.Sprintf("\tADDI %d, SP, SP\n", frameSize))
				}
				if function.Type != "void" {
					switch function.Type {
					case "int64_t", "long":
						builder.WriteString(fmt.Sprintf("\tMOV A0, result+%d(FP)\n", offset))
					case "_Bool":
						builder.WriteString(fmt.Sprintf("\tMOVB A0, result+%d(FP)\n", offset))
					case "double":
						builder.WriteString(fmt.Sprintf("\tMOVD FA0, result+%d(FP)\n", offset))
					case "float":
						builder.WriteString(fmt.Sprintf("\tMOVF FA0, result+%d(FP)\n", offset))
					default:
						return fmt.Errorf("unsupported return type: %v", function.Type)
					}
				}
				builder.WriteString("\tRET\n")
			} else {
				builder.WriteString(formatLineRISCV64(&line))
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
