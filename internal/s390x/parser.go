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
package s390x

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/gorse-io/goat/internal"
	"github.com/klauspost/asmfmt"
	"github.com/samber/lo"
)

const callerStackAreaSize = 160

var (
	attributeLine = regexp.MustCompile(`^\s+\..+$`)
	nameLine      = regexp.MustCompile(`^\w+:.+$`)
	labelLine     = regexp.MustCompile(`^\.\w+_\d+:.*$`)
	codeLine      = regexp.MustCompile(`^\s+\w+.+$`)

	symbolLine = regexp.MustCompile(`^\w+\s+<\w+>:$`)
	dataLine   = regexp.MustCompile(`^\w+:\s+\w+\s+.+$`)

	registers   = []string{"R2", "R3", "R4", "R5", "R6"}
	fpRegisters = []string{"F0", "F2", "F4", "F6"}
)

func init() {
	internal.RegisterTarget("s390x", internal.Target{
		GOARCH:             "s390x",
		BuildTags:          "//go:build !noasm && s390x\n",
		ClangTriple:        "s390x-linux-gnu",
		ObjdumpPath:        "s390x-linux-gnu-objdump",
		ParseAssembly:      parseAssembly,
		ParseObjectDump:    parseObjectDump,
		GenerateGoAssembly: generateGoAssembly,
	})
}

func generateLine(line internal.Line) string {
	var builder strings.Builder
	builder.WriteString("\t")
	for i := 0; i < len(line.Binary); i++ {
		if i > 0 {
			builder.WriteString("\n\t")
		}
		builder.WriteString(fmt.Sprintf("BYTE $0x%02x", line.Binary[i]))
	}
	builder.WriteString("\t// ")
	builder.WriteString(line.Assembly)
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
		switch {
		case attributeLine.MatchString(line):
			continue
		case nameLine.MatchString(line):
			functionName = strings.Split(line, ":")[0]
			functions[functionName] = make([]internal.Line, 0)
			labelName = ""
		case labelLine.MatchString(line):
			labelName = strings.TrimPrefix(strings.Split(line, ":")[0], ".")
			lines := functions[functionName]
			if len(lines) > 0 && lines[len(lines)-1].Assembly == "" {
				lines[len(lines)-1].Labels = append(lines[len(lines)-1].Labels, labelName)
			} else {
				functions[functionName] = append(functions[functionName], internal.Line{Labels: []string{labelName}})
			}
		case codeLine.MatchString(line):
			asm := sanitizeAsm(line)
			if strings.HasPrefix(asm, "aghi") && strings.Contains(asm, "%r15") {
				fields := strings.FieldsFunc(asm, func(r rune) bool {
					return unicode.IsSpace(r) || r == ','
				})
				if len(fields) >= 3 && strings.HasPrefix(fields[2], "-") {
					if size, convErr := strconv.Atoi(strings.TrimPrefix(fields[2], "-")); convErr == nil {
						stackSizes[functionName] = size
					}
				}
			}
			if labelName == "" {
				functions[functionName] = append(functions[functionName], internal.Line{Assembly: asm})
			} else {
				lines := functions[functionName]
				if len(lines) == 0 {
					functions[functionName] = append(functions[functionName], internal.Line{Labels: []string{labelName}})
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

func sanitizeAsm(asm string) string {
	asm = strings.TrimSpace(asm)
	asm = strings.Split(asm, "//")[0]
	asm = strings.TrimSpace(asm)
	return asm
}

func parseObjectDump(dump string, functions map[string][]internal.Line) error {
	var (
		functionName string
		lineNumber   int
	)
	for i, line := range strings.Split(dump, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case symbolLine.MatchString(line):
			functionName = strings.Split(line, "<")[1]
			functionName = strings.Split(functionName, ">")[0]
			lineNumber = 0
		case dataLine.MatchString(line):
			data := strings.TrimSpace(strings.Split(line, ":")[1])
			splits := strings.Split(data, " ")
			var (
				binary   strings.Builder
				assembly string
			)
			for i, s := range splits {
				if s == "" || unicode.IsSpace(rune(s[0])) {
					assembly = sanitizeAsm(strings.Join(splits[i:], " "))
					break
				}
				decoded, err := hex.DecodeString(s)
				if err != nil {
					return fmt.Errorf("%d: invalid s390x instruction bytes %q: %w", i, s, err)
				}
				binary.Write(decoded)
			}
			if assembly == "" {
				return fmt.Errorf("try to increase --insn-width of objdump")
			}
			if strings.Contains(assembly, "nop") {
				continue
			}
			if lineNumber >= len(functions[functionName]) {
				return fmt.Errorf("%d: unexpected objectdump line: %s", i, line)
			}
			functions[functionName][lineNumber].Binary = binary.String()
			lineNumber++
		}
	}
	return nil
}

func parameterSize(param internal.Parameter) int {
	if param.Pointer {
		return 8
	}
	return internal.SupportedTypes[param.Type]
}

func resultSize(typ string) int {
	switch typ {
	case "void":
		return 0
	case "_Bool":
		return 1
	case "float":
		return 4
	case "double", "int64_t", "long":
		return 8
	default:
		_, _ = fmt.Fprintln(os.Stderr, "unsupported return type:", typ)
		os.Exit(1)
		return 0
	}
}

func stackSlotValueOffset(param internal.Parameter) int {
	if param.Pointer {
		return 0
	}
	switch param.Type {
	case "_Bool":
		return 7
	case "float":
		return 4
	default:
		return 0
	}
}

func emitStoreFromFP(builder *strings.Builder, param internal.Parameter, srcOffset, dstOffset int) {
	switch {
	case param.Pointer:
		builder.WriteString(fmt.Sprintf("\tMOVD %s+%d(FP), R0\n", param.Name, srcOffset))
		builder.WriteString(fmt.Sprintf("\tMOVD R0, %d(R15)\n", dstOffset))
	case param.Type == "_Bool":
		builder.WriteString(fmt.Sprintf("\tMOVBZ %s+%d(FP), R0\n", param.Name, srcOffset))
		builder.WriteString(fmt.Sprintf("\tMOVBZ R0, %d(R15)\n", dstOffset))
	case param.Type == "float":
		builder.WriteString(fmt.Sprintf("\tMOVWZ %s+%d(FP), R0\n", param.Name, srcOffset))
		builder.WriteString(fmt.Sprintf("\tMOVWZ R0, %d(R15)\n", dstOffset))
	case param.Type == "double" || param.Type == "int64_t" || param.Type == "long":
		builder.WriteString(fmt.Sprintf("\tMOVD %s+%d(FP), R0\n", param.Name, srcOffset))
		builder.WriteString(fmt.Sprintf("\tMOVD R0, %d(R15)\n", dstOffset))
	default:
		_, _ = fmt.Fprintln(os.Stderr, "unsupported stack parameter type:", param.Type)
		os.Exit(1)
	}
}

func generateGoAssembly(buildTags string, header string, goAssemblyPath string, functions []internal.Function) error {
	var builder strings.Builder
	builder.WriteString(buildTags)
	builder.WriteString(header)
	for _, function := range functions {
		var body strings.Builder
		registerCount, fpRegisterCount, offset := 0, 0, 0
		var stack []lo.Tuple2[int, internal.Parameter]
		for _, param := range function.Parameters {
			sz := parameterSize(param)
			if offset%sz != 0 {
				offset += sz - offset%sz
			}
			if !param.Pointer && (param.Type == "double" || param.Type == "float") {
				if fpRegisterCount < len(fpRegisters) {
					if param.Type == "double" {
						body.WriteString(fmt.Sprintf("\tFMOVD %s+%d(FP), %s\n", param.Name, offset, fpRegisters[fpRegisterCount]))
					} else {
						body.WriteString(fmt.Sprintf("\tFMOVS %s+%d(FP), %s\n", param.Name, offset, fpRegisters[fpRegisterCount]))
					}
					fpRegisterCount++
				} else {
					stack = append(stack, lo.Tuple2[int, internal.Parameter]{A: offset, B: param})
				}
			} else {
				if registerCount < len(registers) {
					switch {
					case param.Pointer, param.Type == "double", param.Type == "int64_t", param.Type == "long":
						body.WriteString(fmt.Sprintf("\tMOVD %s+%d(FP), %s\n", param.Name, offset, registers[registerCount]))
					case param.Type == "_Bool":
						body.WriteString(fmt.Sprintf("\tMOVBZ %s+%d(FP), %s\n", param.Name, offset, registers[registerCount]))
					case param.Type == "float":
						body.WriteString(fmt.Sprintf("\tMOVWZ %s+%d(FP), %s\n", param.Name, offset, registers[registerCount]))
					default:
						return fmt.Errorf("unsupported parameter type: %v", param.Type)
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
		resultOffset := offset
		argSize := resultOffset + resultSize(function.Type)

		frameSize := callerStackAreaSize + len(stack)*8
		builder.WriteString(fmt.Sprintf("\nTEXT ·%v(SB), $%d-%d\n",
			function.Name, frameSize, argSize))
		builder.WriteString(body.String())
		if len(stack) > 0 {
			for i := range stack {
				slotBase := callerStackAreaSize + i*8
				emitStoreFromFP(&builder, stack[i].B, stack[i].A, slotBase+stackSlotValueOffset(stack[i].B))
			}
		}
		for _, line := range function.Lines {
			for _, label := range line.Labels {
				builder.WriteString(label)
				builder.WriteString(":\n")
			}
			if strings.HasPrefix(line.Assembly, "br") && strings.Contains(line.Assembly, "%r14") {
				if function.Type != "void" {
					switch function.Type {
					case "int64_t", "long":
						builder.WriteString(fmt.Sprintf("\tMOVD R2, result+%d(FP)\n", resultOffset))
					case "_Bool":
						builder.WriteString(fmt.Sprintf("\tMOVBZ R2, result+%d(FP)\n", resultOffset))
					case "double":
						builder.WriteString(fmt.Sprintf("\tFMOVD F0, result+%d(FP)\n", resultOffset))
					case "float":
						builder.WriteString(fmt.Sprintf("\tFMOVS F0, result+%d(FP)\n", resultOffset))
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
