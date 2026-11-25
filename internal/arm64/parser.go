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
package arm64

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
	dataNameLine  = regexp.MustCompile(`^[.\w$]+:.*$`)
	labelLine     = regexp.MustCompile(`^\.\w+_\d+:.*$`)
	codeLine      = regexp.MustCompile(`^\s+\w+.+$`)
	jmpLine       = regexp.MustCompile(`^(b|b\.\w{2})\t\.\w+_\d+$`)

	symbolLine = regexp.MustCompile(`^\w+\s+<\w+>:$`)
	dataLine   = regexp.MustCompile(`^\w+:\s+\w+\s+.+$`)
	adrpLine   = regexp.MustCompile(`^adrp\s+x([0-9]+), ([.A-Za-z_][.A-Za-z0-9_$]*)$`)
	lo12Line   = regexp.MustCompile(`^add\s+x([0-9]+), x([0-9]+), :lo12:([.A-Za-z_][.A-Za-z0-9_$]*)$`)
	subSpLine  = regexp.MustCompile(`^sub\s+sp, sp, #(0x[0-9a-fA-F]+|[0-9]+)(?:, lsl #([0-9]+))?$`)
	addSpLine  = regexp.MustCompile(`^add\s+sp, sp, #(0x[0-9a-fA-F]+|[0-9]+)(?:, lsl #([0-9]+))?$`)

	registers   = []string{"R0", "R1", "R2", "R3", "R4", "R5", "R6", "R7"}
	fpRegisters = []string{"F0", "F1", "F2", "F3", "F4", "F5", "F6", "F7"}
	dataSymbols []internal.DataSymbol
)

func dataSymbolName(name string) string {
	return internal.GoDataSymbolName(name)
}

func init() {
	internal.RegisterTarget("arm64", internal.Target{
		GOARCH:      "arm64",
		BuildTags:   "//go:build !noasm && arm64\n",
		ClangTriple: "aarch64-linux-gnu",
		// R18 is the "platform register", reserved on the Apple platform.
		// See https://go.dev/doc/asm#arm64
		ClangOptions:       []string{"-ffixed-x18"},
		PrepareAssembly:    prepareAssembly,
		ParseAssembly:      parseAssembly,
		ParseObjectDump:    parseObjectDump,
		GenerateGoAssembly: generateGoAssembly,
	})
}

var (
	spWordRegex     = regexp.MustCompile(`\bsp\b`)
	spZeroRefRegex  = regexp.MustCompile(`\[sp\]`)
	spImmRefRegex   = regexp.MustCompile(`\[sp, #(0x[0-9a-fA-F]+|[0-9]+)\]`)
	addFromSpRegex  = regexp.MustCompile(`^(\s*add\s+x[0-9]+,\s*sp,\s*#)(0x[0-9a-fA-F]+|[0-9]+)(.*)$`)
	subSpRewriteReg = regexp.MustCompile(`^(\s*sub\s+sp,\s*sp,\s*#)(0x[0-9a-fA-F]+|[0-9]+)(.*)$`)
	addSpRewriteReg = regexp.MustCompile(`^(\s*add\s+sp,\s*sp,\s*#)(0x[0-9a-fA-F]+|[0-9]+)(.*)$`)
)

const lrSlotReserve = 16

func prepareAssembly(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(raw), "\n")
	dataSection := false
	funcStart := -1

	rewriteFunc := func(start, end int) {
		if start < 0 || start >= end {
			return
		}
		var codeIndices []int
		for i := start; i < end; i++ {
			line := lines[i]
			if attributeLine.MatchString(line) || labelLine.MatchString(line) || nameLine.MatchString(line) {
				continue
			}
			if codeLine.MatchString(line) {
				codeIndices = append(codeIndices, i)
			}
		}
		if len(codeIndices) == 0 {
			return
		}
		rewritten := make(map[int]string)
		var spDelta int64
		hasSub := false
		for idx, lineIdx := range codeIndices {
			orig := lines[lineIdx]
			asm, comment, hasComment := strings.Cut(orig, "//")
			trimmed := strings.TrimSpace(asm)
			if trimmed == "ret" {
				if spDelta != 0 || idx != len(codeIndices)-1 {
					return
				}
				continue
			}
			if !spWordRegex.MatchString(trimmed) {
				continue
			}
			if matches := subSpLine.FindStringSubmatch(trimmed); matches != nil {
				imm, ok := parseImmediate(matches[1], matches[2])
				if !ok || matches[2] != "" || spDelta != 0 || imm+lrSlotReserve > 4095 {
					return
				}
				spDelta += imm
				hasSub = true
				m := subSpRewriteReg.FindStringSubmatch(asm)
				if m == nil {
					return
				}
				newAsm := fmt.Sprintf("%s%d%s", m[1], imm+lrSlotReserve, m[3])
				if hasComment {
					rewritten[lineIdx] = newAsm + "//" + comment
				} else {
					rewritten[lineIdx] = newAsm
				}
			} else if matches := addSpLine.FindStringSubmatch(trimmed); matches != nil {
				imm, ok := parseImmediate(matches[1], matches[2])
				if !ok || matches[2] != "" || spDelta != imm || imm+lrSlotReserve > 4095 {
					return
				}
				spDelta -= imm
				m := addSpRewriteReg.FindStringSubmatch(asm)
				if m == nil {
					return
				}
				newAsm := fmt.Sprintf("%s%d%s", m[1], imm+lrSlotReserve, m[3])
				if hasComment {
					rewritten[lineIdx] = newAsm + "//" + comment
				} else {
					rewritten[lineIdx] = newAsm
				}
			} else if spZeroRefRegex.MatchString(asm) && !strings.Contains(asm, "[sp]!") {
				newAsm := spZeroRefRegex.ReplaceAllString(asm, fmt.Sprintf("[sp, #%d]", lrSlotReserve))
				if hasComment {
					rewritten[lineIdx] = newAsm + "//" + comment
				} else {
					rewritten[lineIdx] = newAsm
				}
			} else if m := spImmRefRegex.FindStringSubmatch(asm); m != nil && !strings.Contains(asm, "]!") {
				off, err := strconv.ParseInt(m[1], 0, 64)
				if err != nil {
					return
				}
				newAsm := spImmRefRegex.ReplaceAllString(asm, fmt.Sprintf("[sp, #%d]", off+lrSlotReserve))
				if hasComment {
					rewritten[lineIdx] = newAsm + "//" + comment
				} else {
					rewritten[lineIdx] = newAsm
				}
			} else if m := addFromSpRegex.FindStringSubmatch(asm); m != nil {
				off, err := strconv.ParseInt(m[2], 0, 64)
				if err != nil || off+lrSlotReserve > 4095 {
					return
				}
				newAsm := fmt.Sprintf("%s%d%s", m[1], off+lrSlotReserve, m[3])
				if hasComment {
					rewritten[lineIdx] = newAsm + "//" + comment
				} else {
					rewritten[lineIdx] = newAsm
				}
			} else {
				return
			}
		}
		if !hasSub || spDelta != 0 {
			return
		}
		for i, line := range rewritten {
			lines[i] = line
		}
	}

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, ".section") {
			rewriteFunc(funcStart, i)
			funcStart = -1
			dataSection = strings.Contains(trimmed, ".rodata") || strings.Contains(trimmed, ".data") || strings.Contains(trimmed, ".sdata")
		} else if trimmed == ".text" {
			rewriteFunc(funcStart, i)
			funcStart = -1
			dataSection = false
		} else if !dataSection && nameLine.MatchString(line) {
			name, _, _ := strings.Cut(line, ":")
			if !strings.HasPrefix(name, ".") {
				rewriteFunc(funcStart, i)
				funcStart = i + 1
			}
		}
	}
	rewriteFunc(funcStart, len(lines))

	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
}

func parseImmediate(raw, shift string) (int64, bool) {
	val, err := strconv.ParseInt(raw, 0, 64)
	if err != nil {
		return 0, false
	}
	if shift != "" {
		s, err := strconv.ParseUint(shift, 10, 6)
		if err != nil {
			return 0, false
		}
		val <<= s
	}
	return val, true
}

func translateStackAdjustments(lines []internal.Line) map[int]string {
	replacements := make(map[int]string)
	var spDelta int64
	for i, line := range lines {
		asm := line.Assembly
		if matches := subSpLine.FindStringSubmatch(asm); matches != nil {
			imm, ok := parseImmediate(matches[1], matches[2])
			if !ok {
				return nil
			}
			spDelta += imm
			replacements[i] = fmt.Sprintf("\tSUB $%d, RSP\n\tMOVD R30, 0(RSP)\n", imm)
		} else if matches := addSpLine.FindStringSubmatch(asm); matches != nil {
			imm, ok := parseImmediate(matches[1], matches[2])
			if !ok {
				return nil
			}
			spDelta -= imm
			replacements[i] = fmt.Sprintf("\tADD $%d, RSP\n", imm)
		} else if asm == "ret" {
			if spDelta != 0 || i != len(lines)-1 {
				return nil
			}
		}
	}
	if spDelta != 0 {
		return nil
	}
	return replacements
}

func generateLine(line internal.Line) string {
	var builder strings.Builder
	if jmpLine.MatchString(line.Assembly) {
		splits := strings.Split(line.Assembly, "\t")
		instruction := strings.Map(func(r rune) rune {
			if r == '.' {
				return -1
			}
			return unicode.ToUpper(r)
		}, splits[0])
		label := splits[1][1:]
		builder.WriteString(fmt.Sprintf("%s %s\n", instruction, label))
	} else if matches := adrpLine.FindStringSubmatch(line.Assembly); matches != nil {
		builder.WriteString(fmt.Sprintf("	MOVD $%s<>(SB), R%s\n", dataSymbolName(matches[2]), matches[1]))
	} else if lo12Line.MatchString(line.Assembly) {
		// The preceding ADRP is rewritten to load the full Go symbol address.
	} else {
		builder.WriteString("\t")
		builder.WriteString(fmt.Sprintf("WORD $0x%v", line.Binary))
		builder.WriteString("\t// ")
		builder.WriteString(line.Assembly)
		builder.WriteString("\n")
	}
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
		dataName     string
		dataSection  bool
		data         []internal.DataSymbol
		cfiRegex     = regexp.MustCompile(`^\s+\.cfi_def_cfa_offset\s+(\d+)`)
	)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, ".section") {
			dataSection = strings.Contains(trimmed, ".rodata") || strings.Contains(trimmed, ".data") || strings.Contains(trimmed, ".sdata")
			if !dataSection {
				dataName = ""
			}
		} else if trimmed == ".text" {
			dataSection = false
			dataName = ""
		}
		if dataSection && dataNameLine.MatchString(line) {
			name, _, _ := strings.Cut(line, ":")
			dataName = dataSymbolName(name)
			continue
		}
		if parsed, ok, err := internal.ParseDataDirective(line, binary.LittleEndian); err != nil {
			return nil, nil, err
		} else if ok && dataName != "" {
			if len(data) > 0 && data[len(data)-1].Name == dataName {
				data[len(data)-1].Data = append(data[len(data)-1].Data, parsed...)
			} else {
				data = append(data, internal.DataSymbol{Name: dataName, Data: parsed})
			}
		} else if m := cfiRegex.FindStringSubmatch(line); m != nil {
			if offset, err := strconv.Atoi(m[1]); err == nil && offset > stackSizes[functionName] {
				stackSizes[functionName] = offset
			}
			continue
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
				labelName = ""
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
	builder.WriteString(internal.GenerateDataSymbols(dataSymbols, binary.LittleEndian))
	for _, function := range functions {
		resultSize := internal.SupportedTypes[function.Type]
		registerCount, fpRegisterCount, offset := 0, 0, 0
		var stack []lo.Tuple2[int, internal.Parameter]
		var argsBuilder strings.Builder
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
			if !param.Pointer && (param.Type == "float" || param.Type == "double") {
				if fpRegisterCount < len(fpRegisters) {
					if param.Type == "float" {
						argsBuilder.WriteString(fmt.Sprintf("\tFMOVS %s+%d(FP), %s\n", param.Name, offset, fpRegisters[fpRegisterCount]))
					} else {
						argsBuilder.WriteString(fmt.Sprintf("\tFMOVD %s+%d(FP), %s\n", param.Name, offset, fpRegisters[fpRegisterCount]))
					}
					fpRegisterCount++
				} else {
					stack = append(stack, lo.Tuple2[int, internal.Parameter]{A: offset, B: param})
				}
			} else {
				if registerCount < len(registers) {
					if !param.Pointer && param.Type == "_Bool" {
						argsBuilder.WriteString(fmt.Sprintf("	MOVBU %s+%d(FP), %s\n", param.Name, offset, registers[registerCount]))
					} else {
						argsBuilder.WriteString(fmt.Sprintf("	MOVD %s+%d(FP), %s\n", param.Name, offset, registers[registerCount]))
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
		stackOffset := 0
		if len(stack) > 0 {
			for i := 0; i < len(stack); i++ {
				argsBuilder.WriteString(fmt.Sprintf("\tMOVD %s+%d(FP), R8\n", stack[i].B.Name, stack[i].A))
				argsBuilder.WriteString(fmt.Sprintf("\tMOVD R8, %d(RSP)\n", stackOffset))
				if stack[i].B.Pointer {
					stackOffset += 8
				} else {
					stackOffset += internal.SupportedTypes[stack[i].B.Type]
				}
			}
		}
		if stackOffset%8 != 0 {
			stackOffset += 8 - stackOffset%8
		}
		frameSize := stackOffset + function.StackSize
		// Go's assembler requires frame sizes to be aligned to 16 bytes on arm64
		if frameSize%16 != 0 {
			frameSize += 16 - frameSize%16
		}
		builder.WriteString(fmt.Sprintf("\nTEXT ·%v(SB), $%d-%d\n",
			function.Name, frameSize, offset+resultSize))
		builder.WriteString(argsBuilder.String())
		stackAdjustments := translateStackAdjustments(function.Lines)
		for i, line := range function.Lines {
			for _, label := range line.Labels {
				builder.WriteString(label)
				builder.WriteString(":\n")
			}
			if line.Assembly == "ret" {
				if function.Type != "void" {
					switch function.Type {
					case "int64_t", "long":
						builder.WriteString(fmt.Sprintf("	MOVD R0, result+%d(FP)\n", offset))
					case "_Bool":
						builder.WriteString(fmt.Sprintf("	MOVB R0, result+%d(FP)\n", offset))
					case "double":
						builder.WriteString(fmt.Sprintf("\tFMOVD F0, result+%d(FP)\n", offset))
					case "float":
						builder.WriteString(fmt.Sprintf("\tFMOVS F0, result+%d(FP)\n", offset))
					default:
						return fmt.Errorf("unsupported return type: %v", function.Type)
					}
				}
				builder.WriteString("\tRET\n")
			} else if replacement, ok := stackAdjustments[i]; ok {
				builder.WriteString(replacement)
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
