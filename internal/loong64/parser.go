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
package loong64

import (
	"bufio"
	"encoding/binary"
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

var (
	attributeLine = regexp.MustCompile(`^\s+\..+$`)
	nameLine      = regexp.MustCompile(`^\w+:.*$`)
	labelLine     = regexp.MustCompile(`^\.\w+_\d+:.*$`)
	codeLine      = regexp.MustCompile(`^\s+\w+.+$`)

	symbolLine = regexp.MustCompile(`^\w+\s+<\w+>:$`)
	dataLine   = regexp.MustCompile(`^\w+:\s+\w+\s+.+$`)
	pcHiLine   = regexp.MustCompile(`^pcalau12i\s+(\$[a-z0-9]+), %pc_hi20\(([A-Za-z_][A-Za-z0-9_]*)\)$`)
	pcLoLine   = regexp.MustCompile(`^addi\.d\s+(\$[a-z0-9]+), (\$[a-z0-9]+), %pc_lo12\(([A-Za-z_][A-Za-z0-9_]*)\)$`)

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
		"$fp":   "R21",
		"$s0":   "R23",
		"$s1":   "R24",
		"$s2":   "R25",
		"$s3":   "R26",
		"$s4":   "R27",
		"$s5":   "R28",
		"$s6":   "R29",
		"$s7":   "R30",
		"$s8":   "R31",
		"$s9":   "R21",
	}
	opAlias = map[string]string{
		"b":    "JMP",
		"bnez": "BNE",
	}
	dataSymbols []internal.DataSymbol
)

func init() {
	internal.RegisterTarget("loong64", internal.Target{
		GOARCH:             "loong64",
		BuildTags:          "//go:build !noasm && loong64\n",
		ClangTriple:        "loongarch64-linux-gnu",
		ParseAssembly:      parseAssembly,
		ParseObjectDump:    parseObjectDump,
		GenerateGoAssembly: generateGoAssembly,
	})
}

func generateLine(line internal.Line) string {
	var builder strings.Builder
	builder.WriteString("\t")
	if matches := pcHiLine.FindStringSubmatch(line.Assembly); matches != nil {
		if r, ok := registersAlias[matches[1]]; !ok {
			_, _ = fmt.Fprintln(os.Stderr, "unexpected register alias:", matches[1])
			os.Exit(1)
		} else {
			builder.WriteString(fmt.Sprintf("MOVV $%s<>(SB), %s", matches[2], r))
		}
	} else if pcLoLine.MatchString(line.Assembly) {
		// The preceding PCALAU12I is rewritten to load the full Go symbol address.
	} else if branch, ok := translateBranch(line.Assembly); ok {
		builder.WriteString(branch)
	} else {
		builder.WriteString("\t")
		builder.WriteString(fmt.Sprintf("WORD $0x%v", line.Binary))
		builder.WriteString("\t// ")
		builder.WriteString(line.Assembly)
	}
	builder.WriteString("\n")
	return builder.String()
}

func translateBranch(asm string) (string, bool) {
	fields := strings.Fields(strings.TrimSpace(asm))
	if len(fields) == 0 || !strings.HasPrefix(fields[0], "b") {
		return "", false
	}
	mnemonic := fields[0]
	rest := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(asm), mnemonic))
	operands := strings.Split(rest, ",")
	for i := range operands {
		operands[i] = strings.TrimSpace(operands[i])
	}
	register := func(alias string) string {
		if r, ok := registersAlias[alias]; ok {
			return r
		}
		_, _ = fmt.Fprintln(os.Stderr, "unexpected register alias:", alias)
		os.Exit(1)
		return ""
	}
	label := func(s string) string {
		return strings.TrimPrefix(strings.TrimSpace(s), ".")
	}

	switch mnemonic {
	case "b":
		if len(operands) != 1 {
			return "", false
		}
		return fmt.Sprintf("JMP %s", label(operands[0])), true
	case "beqz":
		if len(operands) != 2 {
			return "", false
		}
		return fmt.Sprintf("BEQ %s, R0, %s", register(operands[0]), label(operands[1])), true
	case "bnez":
		if len(operands) != 2 {
			return "", false
		}
		return fmt.Sprintf("BNE %s, R0, %s", register(operands[0]), label(operands[1])), true
	case "beq", "bne", "blt", "bge", "bltu", "bgeu":
		if len(operands) != 3 {
			return "", false
		}
		return fmt.Sprintf("%s %s, %s, %s", strings.ToUpper(mnemonic), register(operands[0]), register(operands[1]), label(operands[2])), true
	default:
		return "", false
	}
}

func rewriteReservedRegister(binaryText, asm string) (string, error) {
	if !strings.Contains(asm, "$fp") && !strings.Contains(asm, "$s9") {
		return binaryText, nil
	}
	instruction, err := strconv.ParseUint(binaryText, 16, 32)
	if err != nil {
		return "", err
	}
	for _, shift := range []uint{0, 5, 10} {
		if (instruction>>shift)&0x1f == 22 {
			instruction = (instruction & ^(uint64(0x1f) << shift)) | (uint64(21) << shift)
		}
	}
	return fmt.Sprintf("%08x", instruction), nil
}

func argumentSize(function internal.Function, returnSize int) int {
	offset := 0
	for _, param := range function.Parameters {
		sz := 8
		if !param.Pointer {
			sz = internal.SupportedTypes[param.Type]
		}
		if offset%sz != 0 {
			offset += sz - offset%sz
		}
		offset += sz
	}
	if offset%8 != 0 {
		offset += 8 - offset%8
	}
	return offset + returnSize
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
		dataName     string
		dataSection  bool
		data         []internal.DataSymbol
	)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, ".section") {
			dataSection = strings.Contains(trimmed, ".rodata") || strings.Contains(trimmed, ".data")
		} else if trimmed == ".text" {
			dataSection = false
		}
		if parsed, ok, err := internal.ParseDataDirective(line); err != nil {
			return nil, nil, err
		} else if ok && dataName != "" {
			data = append(data, internal.DataSymbol{Name: dataName, Data: parsed})
			dataName = ""
		} else if attributeLine.MatchString(line) {
			continue
		} else if nameLine.MatchString(line) {
			name, _, _ := strings.Cut(line, ":")
			if strings.HasPrefix(name, ".") {
				continue
			}
			if dataSection {
				dataName = name
			} else {
				functionName = name
				functions[functionName] = make([]internal.Line, 0)
			}
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
			asm, _, _ := strings.Cut(line, "//")
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
	dataSymbols = data
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
			if assembly == "nop" {
				continue
			}
			if lineNumber >= len(functions[functionName]) {
				return fmt.Errorf("%d: unexpected objectdump line: %s", i, line)
			}
			rewritten, err := rewriteReservedRegister(binary, assembly)
			if err != nil {
				return err
			}
			binary = rewritten
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
	builder.WriteString(internal.GenerateDataSymbols(dataSymbols, binary.LittleEndian))
	for _, function := range functions {
		returnSize := 0
		if function.Type != "void" {
			returnSize += 8
		}
		resultSize := internal.SupportedTypes[function.Type]
		builder.WriteString(fmt.Sprintf("\nTEXT ·%v(SB), $%d-%d\n",
			function.Name, returnSize, argumentSize(function, resultSize)))
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
					if !param.Pointer && param.Type == "_Bool" {
						builder.WriteString(fmt.Sprintf("\tMOVBU %s+%d(FP), %s\n", param.Name, offset, registers[registerCount]))
					} else {
						builder.WriteString(fmt.Sprintf("\tMOVV %s+%d(FP), %s\n", param.Name, offset, registers[registerCount]))
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
			stackoffset := 0
			for i := 0; i < len(stack); i++ {
				if i > 0 {
					builder.WriteString(fmt.Sprintf("	ADDV $%d, R3\n", frameSize))
				}
				builder.WriteString(fmt.Sprintf("	MOVV %s+%d(FP), R12\n", stack[i].B.Name, stack[i].A))
				builder.WriteString(fmt.Sprintf("	ADDV $-%d, R3\n", frameSize))
				builder.WriteString(fmt.Sprintf("	MOVV R12, (%d)(R3)\n", stackoffset))
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
					builder.WriteString(fmt.Sprintf("\tADDV $%d, R3\n", frameSize))
				}
				if function.Type != "void" {
					switch function.Type {
					case "int64_t", "long":
						builder.WriteString(fmt.Sprintf("	MOVV R4, result+%d(FP)\n", offset))
					case "_Bool":
						builder.WriteString(fmt.Sprintf("	MOVB R4, result+%d(FP)\n", offset))
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
