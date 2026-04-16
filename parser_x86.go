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
	"fmt"
	"os"
	"strings"

	"github.com/gorse-io/goat/internal/types"
	"github.com/klauspost/asmfmt"
	"github.com/samber/lo"
)

var (
	amd64Registers    = []string{"DI", "SI", "DX", "CX", "R8", "R9"}
	amd64XmmRegisters = []string{"X0", "X1", "X2", "X3", "X4", "X5", "X6", "X7"}
)

func formatLineAMD64(line *Line) string {
	var builder strings.Builder
	builder.WriteString("\t")
	if strings.HasPrefix(line.Assembly, "j") {
		splits := strings.Split(line.Assembly, ".")
		op := strings.TrimSpace(splits[0])
		operand := splits[1]
		builder.WriteString(fmt.Sprintf("%s %s", strings.ToUpper(op), operand))
	} else {
		pos := 0
		for pos < len(line.Binary) {
			if pos > 0 {
				builder.WriteString("; ")
			}
			if len(line.Binary)-pos >= 8 {
				builder.WriteString(fmt.Sprintf("QUAD $0x%v%v%v%v%v%v%v%v",
					line.Binary[pos+7], line.Binary[pos+6], line.Binary[pos+5], line.Binary[pos+4],
					line.Binary[pos+3], line.Binary[pos+2], line.Binary[pos+1], line.Binary[pos]))
				pos += 8
			} else if len(line.Binary)-pos >= 4 {
				builder.WriteString(fmt.Sprintf("LONG $0x%v%v%v%v",
					line.Binary[pos+3], line.Binary[pos+2], line.Binary[pos+1], line.Binary[pos]))
				pos += 4
			} else if len(line.Binary)-pos >= 2 {
				builder.WriteString(fmt.Sprintf("WORD $0x%v%v", line.Binary[pos+1], line.Binary[pos]))
				pos += 2
			} else {
				builder.WriteString(fmt.Sprintf("BYTE $0x%v", line.Binary[pos]))
				pos += 1
			}
		}
		builder.WriteString("\t// ")
		builder.WriteString(line.Assembly)
	}
	builder.WriteString("\n")
	return builder.String()
}

func (t *TranslateUnit) generateGoAssemblyX86(path string, functions []Function) error {
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
		registerIndex, xmmRegisterIndex, offset := 0, 0, 0
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
				if xmmRegisterIndex < len(amd64XmmRegisters) {
					if param.Type == "double" {
						builder.WriteString(fmt.Sprintf("\tMOVSD %s+%d(FP), %s\n", param.Name, offset, amd64XmmRegisters[xmmRegisterIndex]))
					} else {
						builder.WriteString(fmt.Sprintf("\tMOVSS %s+%d(FP), %s\n", param.Name, offset, amd64XmmRegisters[xmmRegisterIndex]))
					}
					xmmRegisterIndex++
				} else {
					stack = append(stack, lo.Tuple2[int, Parameter]{A: offset, B: param})
				}
			} else {
				if registerIndex < len(amd64Registers) {
					builder.WriteString(fmt.Sprintf("\tMOVQ %s+%d(FP), %s\n", param.Name, offset, amd64Registers[registerIndex]))
					registerIndex++
				} else {
					stack = append(stack, lo.Tuple2[int, Parameter]{A: offset, B: param})
				}
			}
			offset += sz
		}
		if offset%8 != 0 {
			offset += 8 - offset%8
		}
		if len(stack) > 0 {
			for i := len(stack) - 1; i >= 0; i-- {
				builder.WriteString(fmt.Sprintf("\tPUSHQ %s+%d(FP)\n", stack[i].B.Name, stack[i].A))
			}
			builder.WriteString("\tPUSHQ $0\n")
		}
		for _, line := range function.Lines {
			for _, label := range line.Labels {
				builder.WriteString(label)
				builder.WriteString(":\n")
			}
			if line.Assembly == "retq" {
				if len(stack) > 0 {
					for i := 0; i <= len(stack); i++ {
						builder.WriteString("\tPOPQ DI\n")
					}
				}
				if function.Type != "void" {
					switch function.Type {
					case "int64_t", "long", "_Bool":
						builder.WriteString(fmt.Sprintf("\tMOVQ AX, result+%d(FP)\n", offset))
					case "double":
						builder.WriteString(fmt.Sprintf("\tMOVSD X0, result+%d(FP)\n", offset))
					case "float":
						builder.WriteString(fmt.Sprintf("\tMOVSS X0, result+%d(FP)\n", offset))
					default:
						return fmt.Errorf("unsupported return type: %v", function.Type)
					}
				}
				builder.WriteString("\tRET\n")
			} else {
				builder.WriteString(formatLineAMD64(&line))
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