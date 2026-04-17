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
	"unicode"

	"github.com/gorse-io/goat/internal"
	"github.com/klauspost/asmfmt"
	"github.com/samber/lo"
)

var (
	attributeLine = regexp.MustCompile(`^\s+\..+$`)
	nameLine      = regexp.MustCompile(`^\w+:.+$`)
	labelLine     = regexp.MustCompile(`^\.\w+_\d+:.*$`)
	codeLine      = regexp.MustCompile(`^\s+\w+.+$`)

	symbolLine = regexp.MustCompile(`^\w+\s+<\w+>:$`)
	dataLine   = regexp.MustCompile(`^\w+:\s+\w+\s+.+$`)

	registers   = []string{"A0", "A1", "A2", "A3", "A4", "A5", "A6", "A7"}
	fpRegisters = []string{"FA0", "FA1", "FA2", "FA3", "FA4", "FA5", "FA6", "FA7"}
)

func init() {
	var prologue strings.Builder
	prologue.WriteString("#define __riscv_vector 1\n")
	for _, typeStr := range []string{"int64", "uint64", "int32", "uint32", "int16", "uint16", "int8", "uint8", "float64", "float32", "float16"} {
		for i := 1; i <= 8; i *= 2 {
			prologue.WriteString(fmt.Sprintf("typedef char v%sm%d_t;\n", typeStr, i))
		}
	}

	internal.RegisterTarget("riscv64", internal.Target{
		GOARCH:      "riscv64",
		BuildTags:   "//go:build !noasm && riscv64\n",
		ClangTriple: "riscv64-linux-gnu",
		Prologue:    prologue.String(),
		// X27 points to the Go routine structure.
		ClangOptions:       []string{"-ffixed-x27"},
		ParseAssembly:      parseAssembly,
		ParseObjectDump:    parseObjectDump,
		GenerateGoAssembly: generateGoAssembly,
	})
}

func generateLine(line internal.Line) string {
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

func parseAssembly(path string) (map[string][]internal.Line, map[string]int, error) {
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
		functions    = make(map[string][]internal.Line)
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
			functions[functionName] = make([]internal.Line, 0)
		} else if labelLine.MatchString(line) {
			labelName = strings.Split(line, ":")[0]
			labelName = labelName[1:]
			lines := functions[functionName]
			if len(lines) == 1 || lines[len(lines)-1].Assembly != "" {
				functions[functionName] = append(functions[functionName], internal.Line{Labels: []string{labelName}})
			} else {
				lines[len(lines)-1].Labels = append(lines[len(lines)-1].Labels, labelName)
			}
		} else if codeLine.MatchString(line) {
			asm := strings.Split(line, "//")[0]
			asm = strings.TrimSpace(asm)
			if labelName == "" {
				functions[functionName] = append(functions[functionName], internal.Line{Assembly: asm})
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

func parseObjectDump(dump string, functions map[string][]internal.Line) error {
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
			if lineNumber >= len(functions[functionName]) {
				return fmt.Errorf("%d: unexpected objectdump line: %s", i, line)
			}
			functions[functionName][lineNumber].Binary = binary
			lineNumber++
		}
	}
	return nil
}

func generateGoAssembly(buildTags string, header string, goAssemblyPath string, functions []internal.Function) error {
	// generate code
	var builder strings.Builder
	builder.WriteString(buildTags)
	builder.WriteString(header)
	for _, function := range functions {
		returnSize := 0
		if function.Type != "void" {
			returnSize += 8
		}
		builder.WriteString(fmt.Sprintf("\nTEXT ·%v(SB), $%d-%d\n",
			function.Name, returnSize, len(function.Parameters)*8))
		registerCount, fpRegisterCount, offset := 0, 0, 0
		var stack []lo.Tuple2[int, internal.Parameter]
		for _, param := range function.Parameters {
			sz := 8
			if param.Pointer {
				sz = 8
			} else {
				sz = internal.SupportedTypes[param.Type]
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
					stack = append(stack, lo.Tuple2[int, internal.Parameter]{A: offset, B: param})
				}
			} else {
				if registerCount < len(registers) {
					if param.Type == "_Bool" {
						builder.WriteString(fmt.Sprintf("\tMOVB %s+%d(FP), %s\n", param.Name, offset, registers[registerCount]))
					} else {
						builder.WriteString(fmt.Sprintf("\tMOV %s+%d(FP), %s\n", param.Name, offset, registers[registerCount]))
					}
					registerCount++
				} else {
					stack = append(stack, lo.Tuple2[int, internal.Parameter]{A: offset, B: param})
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
					frameSize += internal.SupportedTypes[stack[i].B.Type]
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
					stackoffset += internal.SupportedTypes[stack[i].B.Type]
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
				builder.WriteString(generateLine(line))
			}
		}
	}

	// write file
	f, err := os.Create(goAssemblyPath)
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
