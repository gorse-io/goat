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
)

const buildTags = "//go:build !noasm && amd64\n"

var (
	attributeLine = regexp.MustCompile(`^\s+\..+$`)
	nameLine      = regexp.MustCompile(`^\w+:.+$`)
	labelLine     = regexp.MustCompile(`^\.\w+_\d+:.*$`)
	codeLine      = regexp.MustCompile(`^\s+\w+.+$`)

	symbolLine = regexp.MustCompile(`^\w+\s+<\w+>:$`)
	dataLine   = regexp.MustCompile(`^\w+:\s+\w+\s+.+$`)

	registers = []string{"DI", "SI", "DX", "CX", "R8", "R9", "R10", "R11"}
)

type Line struct {
	Labels   []string
	Assembly string
	Binary   []string
}

func (line *Line) String() string {
	var builder strings.Builder
	for _, label := range line.Labels {
		builder.WriteString(label)
		builder.WriteString(":\n")
	}
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

func parseAssembly(path string) (map[string][]Line, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func(file *os.File) {
		if err = file.Close(); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}(file)

	var (
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
			labelName = ""
		} else if labelLine.MatchString(line) {
			labelName = strings.Split(line, ":")[0]
			labelName = labelName[1:]
			lines := functions[functionName]
			if len(lines) > 0 && lines[len(lines)-1].Assembly == "" {
				// If the last line is a label, append the label to the last line.
				lines[len(lines)-1].Labels = append(lines[len(lines)-1].Labels, labelName)
			} else {
				functions[functionName] = append(functions[functionName], Line{Labels: []string{labelName}})
			}
		} else if codeLine.MatchString(line) {
			asm := sanitizeAsm(line)
			if labelName == "" {
				functions[functionName] = append(functions[functionName], Line{Assembly: asm})
			} else {
				lines := functions[functionName]
				if len(lines) == 0 {
					functions[functionName] = append(functions[functionName], Line{Labels: []string{labelName}})
					lines = functions[functionName]
				}

				lines[len(lines)-1].Assembly = asm
				labelName = ""
			}
		}
	}

	if err = scanner.Err(); err != nil {
		return nil, err
	}
	return functions, nil
}

func sanitizeAsm(asm string) string {
	asm = strings.TrimSpace(asm)
	asm = strings.Split(asm, "//")[0]
	asm = strings.TrimSpace(asm)

	return asm
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
				binary   []string
				assembly string
			)
			for i, s := range splits {
				if s == "" || unicode.IsSpace(rune(s[0])) {
					assembly = strings.Join(splits[i:], " ")
					assembly = strings.TrimSpace(assembly)
					break
				}
				binary = append(binary, s)
			}

			assembly = sanitizeAsm(assembly)
			if strings.Contains(assembly, "nop") {
				continue
			}

			if assembly == "" {
				return fmt.Errorf("try to increase --insn-width of objdump")
			} else if strings.HasPrefix(assembly, "nop") ||
				assembly == "xchg   %ax,%ax" ||
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
			function.Name, returnSize, returnSize+len(function.Parameters)*8))
		for i, param := range function.Parameters {
			builder.WriteString(fmt.Sprintf("\tMOVQ %s+%d(FP), %s\n", param.Name, i*8, registers[i]))
		}
		for _, line := range function.Lines {
			if line.Assembly == "retq" && function.Type != "void" {
				switch function.Type {
				case "int64_t", "long", "_Bool":
					builder.WriteString(fmt.Sprintf("\tMOVQ AX, result+%d(FP)\n", len(function.Parameters)*8))
				case "double":
					builder.WriteString(fmt.Sprintf("\tMOVSD X0, result+%d(FP)\n", len(function.Parameters)*8))
				case "float":
					builder.WriteString(fmt.Sprintf("\tMOVSS X0, result+%d(FP)\n", len(function.Parameters)*8))
				default:
					return fmt.Errorf("unsupported return type: %v", function.Type)
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
