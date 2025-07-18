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
	buildTags   = "//go:build !noasm && loong64\n"
	buildTarget = "loongarch64-linux-gnu"
)

var (
	attributeLine = regexp.MustCompile(`^\s+\..+$`)
	nameLine      = regexp.MustCompile(`^\w+:.+$`)
	labelLine     = regexp.MustCompile(`^\.\w+_\d+:.*$`)
	codeLine      = regexp.MustCompile(`^\s+\w+.+$`)

	symbolLine = regexp.MustCompile(`^\w+\s+<\w+>:$`)
	dataLine   = regexp.MustCompile(`^\w+:\s+\w+\s+.+$`)

	registers   = []string{"R4", "R5", "R6", "R7", "R8", "R9", "R10", "R11"}
	fpRegisters = []string{"F0", "F1", "F2", "F3", "F4", "F5", "F6", "F7"}

	registersAlias = map[string]string{
		"$zero": "R0",
		"$ra":   "R1",
		"$tp":   "R2",
		"$sp":   "R3",
		"$a0":   "R4",
		"$a1":   "R5",
		"$a2":   "R6",
		"$a3":   "R7",
		"$a4":   "R8",
		"$a5":   "R9",
		"$a6":   "R10",
		"$a7":   "R11",
		"$t0":   "R12",
		"$t1":   "R13",
		"$t2":   "R14",
		"$t3":   "R15",
		"$t4":   "R16",
		"$t5":   "R17",
		"$t6":   "R18",
		"$t7":   "R19",
		"$t8":   "R20",
		"$fp":   "R22",
		"$s0":   "R23",
		"$s1":   "R24",
		"$s2":   "R25",
		"$s3":   "R26",
		"$s4":   "R27",
		"$s5":   "R28",
		"$s6":   "R29",
		"$s7":   "R30",
		"$s8":   "R31",
		"$s9":   "R22",
	}
	opAlias = map[string]string{
		"b":    "JMP",
		"bnez": "BNE",
	}
)

type Line struct {
	Labels   []string
	Assembly string
	Binary   string
}

func (line *Line) String() string {
	var builder strings.Builder
	builder.WriteString("\t")
	if strings.HasPrefix(line.Assembly, "b") && !strings.HasPrefix(line.Assembly, "bstrins") {
		splits := strings.Split(line.Assembly, ".")
		op := strings.TrimSpace(splits[0])
		registers := strings.FieldsFunc(op, func(r rune) bool {
			return unicode.IsSpace(r) || r == ','
		})
		if o, ok := opAlias[registers[0]]; !ok {
			builder.WriteString(strings.ToUpper(registers[0]))
		} else {
			builder.WriteString(o)
		}
		builder.WriteRune(' ')
		for i := 1; i < len(registers); i++ {
			if r, ok := registersAlias[registers[i]]; !ok {
				_, _ = fmt.Fprintln(os.Stderr, "unexpected register alias:", registers[i])
				os.Exit(1)
			} else {
				builder.WriteString(r)
				builder.WriteRune(',')
			}
		}
		builder.WriteString(splits[1])
	} else {
		builder.WriteString("\t")
		builder.WriteString(fmt.Sprintf("WORD $0x%v", line.Binary))
		builder.WriteString("\t// ")
		builder.WriteString(line.Assembly)
	}
	builder.WriteString("\n")
	return builder.String()
}

func parseAssembly(path string) (map[string][]Line, map[string]int, error) {
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
		if attributeLine.MatchString(line) {
			continue
		} else if nameLine.MatchString(line) {
			functionName = strings.Split(line, ":")[0]
			functions[functionName] = make([]Line, 0)
		} else if labelLine.MatchString(line) {
			labelName = strings.Split(line, ":")[0]
			labelName = labelName[1:]
			lines := functions[functionName]
			if len(lines) == 1 || lines[len(lines)-1].Assembly != "" {
				functions[functionName] = append(functions[functionName], Line{Labels: []string{labelName}})
			} else {
				lines[len(lines)-1].Labels = append(lines[len(lines)-1].Labels, labelName)
			}
		} else if codeLine.MatchString(line) {
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

func parseObjectDump(dump string, functions map[string][]Line) error {
	var (
		functionName string
		lineNumber   int
	)
	for i, line := range strings.Split(dump, "\n") {
		line = strings.TrimSpace(line)
		if symbolLine.MatchString(line) {
			functionName = strings.Split(line, "<")[1]
			functionName = strings.Split(functionName, ">")[0]
			lineNumber = 0
		} else if dataLine.MatchString(line) {
			data := strings.Split(line, ":")[1]
			data = strings.TrimSpace(data)
			splits := strings.Split(data, " ")
			var (
				binary   string
				assembly string
			)
			for i, s := range splits {
				if s == "" || unicode.IsSpace(rune(s[0])) {
					assembly = strings.Join(splits[i:], " ")
					assembly = strings.TrimSpace(assembly)
					break
				}
				binary = s
			}
			if assembly == "nop" {
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

func (t *TranslateUnit) generateGoAssembly(path string, functions []Function) error {
	// generate code
	var builder strings.Builder
	builder.WriteString(buildTags)
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
				sz = supportedTypes[param.Type]
			}
			if offset%sz != 0 {
				offset += sz - offset%sz
			}
			if !param.Pointer && (param.Type == "double" || param.Type == "float") {
				if fpRegisterCount < len(fpRegisters) {
					if param.Type == "double" {
						builder.WriteString(fmt.Sprintf("\tMOVD %s+%d(FP), %s\n", param.Name, offset, fpRegisters[fpRegisterCount]))
					} else {
						builder.WriteString(fmt.Sprintf("\tMOVF %s+%d(FP), %s\n", param.Name, offset, fpRegisters[fpRegisterCount]))
					}
					fpRegisterCount++
				} else {
					stack = append(stack, lo.Tuple2[int, Parameter]{A: offset, B: param})
				}
			} else {
				if registerCount < len(registers) {
					builder.WriteString(fmt.Sprintf("\tMOVV %s+%d(FP), %s\n", param.Name, offset, registers[registerCount]))
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
				frameSize += supportedTypes[stack[i].B.Type]
			}
			builder.WriteString(fmt.Sprintf("\tADDV $-%d, R3\n", frameSize))
			stackoffset := 0
			for i := 0; i < len(stack); i++ {
				builder.WriteString(fmt.Sprintf("\tMOVV %s+%d(FP), R12\n", stack[i].B.Name, frameSize+stack[i].A))
				builder.WriteString(fmt.Sprintf("\tMOVV R12, (%d)(R3)\n", stackoffset))
				stackoffset += supportedTypes[stack[i].B.Type]
			}
		}
		for _, line := range function.Lines {
			for _, label := range line.Labels {
				builder.WriteString(label)
				builder.WriteString(":\n")
			}
			if line.Assembly == "ret" {
				if frameSize > 0 {
					builder.WriteString(fmt.Sprintf("\tADDV $%d, R3\n", frameSize))
				}
				if function.Type != "void" {
					switch function.Type {
					case "int64_t", "long", "_Bool":
						builder.WriteString(fmt.Sprintf("\tMOVV R4, result+%d(FP)\n", offset))
					case "double":
						builder.WriteString(fmt.Sprintf("\tMOVD F0, result+%d(FP)\n", offset))
					case "float":
						builder.WriteString(fmt.Sprintf("\tMOVF F0, result+%d(FP)\n", offset))
					default:
						return fmt.Errorf("unsupported return type: %v", function.Type)
					}
				}
				builder.WriteString("\tRET\n")
			} else {
				builder.WriteString(line.String())
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
