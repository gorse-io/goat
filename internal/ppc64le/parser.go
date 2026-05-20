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
package ppc64le

import (
	"bufio"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/gorse-io/goat/internal"
	"github.com/klauspost/asmfmt"
)

var (
	attributeLine    = regexp.MustCompile(`^\s+\..+$`)
	nameLine         = regexp.MustCompile(`^\w+:.*$`)
	labelLine        = regexp.MustCompile(`^\.L[\w$]*:.*$`)
	codeLine         = regexp.MustCompile(`^\s+\w+.+$`)
	stackRefLine     = regexp.MustCompile(`-(\d+)\(([rR]?1)\)`)
	stackMoveLine    = regexp.MustCompile(`^(std|ld|stw|lwz)\s+r(\d+),(-\d+)\(r1\)$`)
	overflowLoadLine = regexp.MustCompile(`^ld\s+r(\d+),(\d+)\(r1\)$`)
	registerLine     = regexp.MustCompile(`\br(\d+)\b`)
	tocHighLine      = regexp.MustCompile(`^addis\s+r?(\d+),r?2,([.A-Za-z_][.A-Za-z0-9_]*)@toc@ha$`)
	tocLowLine       = regexp.MustCompile(`^addi\s+r?(\d+),r?(\d+),([.A-Za-z_][.A-Za-z0-9_]*)@toc@l$`)
	anchorSetLine    = regexp.MustCompile(`^\.set\s+(\.L[A-Za-z0-9_]+),\s*\.\s*\+\s*0$`)
	numericLabelLine = regexp.MustCompile(`^\d+:\s+(.+)$`)

	symbolLine = regexp.MustCompile(`^[0-9a-f]+\s+<\w+>:$`)
	dataLine   = regexp.MustCompile(`^[0-9a-f]+:\s+[0-9a-f]{2}(?:\s+[0-9a-f]{2}){3}.*$`)

	registers   = []string{"R3", "R4", "R5", "R6", "R7", "R8", "R9", "R10"}
	fpRegisters = []string{"F1", "F2", "F3", "F4", "F5", "F6", "F7", "F8", "F9", "F10", "F11", "F12", "F13"}
	dataSymbols []internal.DataSymbol
	dataAnchors = make(map[string]string)
)

const ppc64LinkageSize = 32

func init() {
	internal.RegisterTarget("ppc64le", internal.Target{
		GOARCH:             "ppc64le",
		BuildTags:          "//go:build !noasm && ppc64le\n",
		ClangTriple:        "powerpc64le-linux-gnu",
		ClangOptions:       []string{"-O1"},
		ParseAssembly:      parseAssembly,
		ParseObjectDump:    parseObjectDump,
		GenerateGoAssembly: generateGoAssembly,
	})
}

func generateLine(line internal.Line) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("\tWORD $0x%02x%02x%02x%02x",
		line.Binary[3], line.Binary[2], line.Binary[1], line.Binary[0]))
	builder.WriteString("\t// ")
	builder.WriteString(line.Assembly)
	builder.WriteByte('\n')
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
		stackSizes    = make(map[string]int)
		functions     = make(map[string][]internal.Line)
		functionName  string
		labelName     string
		dataName      string
		dataSection   bool
		pendingAnchor string
		data          []internal.DataSymbol
		anchors       = make(map[string]string)
	)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if matches := numericLabelLine.FindStringSubmatch(strings.TrimSpace(line)); matches != nil {
			line = "\t" + matches[1]
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, ".section") {
			dataSection = strings.Contains(trimmed, ".rodata") || strings.Contains(trimmed, ".data")
		}
		switch {
		case func() bool {
			parsed, ok, err := internal.ParseDataDirective(line)
			if err != nil {
				return false
			}
			if ok && dataName != "" {
				if len(data) > 0 && data[len(data)-1].Name == dataName {
					data[len(data)-1].Data = append(data[len(data)-1].Data, parsed...)
				} else {
					data = append(data, internal.DataSymbol{Name: dataName, Data: parsed})
				}
				return true
			}
			return false
		}():
			continue
		case anchorSetLine.MatchString(trimmed):
			pendingAnchor = anchorSetLine.FindStringSubmatch(trimmed)[1]
		case attributeLine.MatchString(line):
			continue
		case nameLine.MatchString(line):
			name, _, _ := strings.Cut(line, ":")
			if strings.HasPrefix(name, ".") {
				continue
			}
			if dataSection {
				dataName = name
				if pendingAnchor != "" {
					anchors[strings.ToLower(pendingAnchor)] = name
					pendingAnchor = ""
				}
			} else {
				functionName = name
				functions[functionName] = make([]internal.Line, 0)
				labelName = ""
			}
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
	dataSymbols = data
	dataAnchors = anchors
	return functions, stackSizes, nil
}

func sanitizeAsm(asm string) string {
	asm = strings.TrimSpace(asm)
	asm = strings.Split(asm, "#")[0]
	asm = strings.Split(asm, "//")[0]
	asm = strings.TrimSpace(asm)
	return asm
}

func parseObjectDump(dump string, functions map[string][]internal.Line) error {
	var functionName string
	instructionIndexes := make(map[string]int)
	for i, line := range strings.Split(dump, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case symbolLine.MatchString(line):
			functionName = strings.Split(line, "<")[1]
			functionName = strings.Split(functionName, ">")[0]
		case dataLine.MatchString(line):
			lines, ok := functions[functionName]
			if !ok {
				continue
			}
			splits := strings.Fields(line)
			if len(splits) < 6 {
				continue
			}
			var binary strings.Builder
			for _, s := range splits[1:5] {
				decoded, err := hex.DecodeString(s)
				if err != nil {
					return fmt.Errorf("%d: invalid ppc64le instruction bytes %q: %w", i, s, err)
				}
				binary.Write(decoded)
			}
			assembly := sanitizeAsm(strings.Join(splits[5:], " "))
			if assembly == "" {
				continue
			}
			index := instructionIndexes[functionName]
			for index < len(lines) && lines[index].Assembly == "" {
				index++
			}
			if index < len(lines) {
				lines[index].Binary = binary.String()
				// Keep the compiler assembly text so relocatable references such as
				// symbol@toc@ha/symbol@toc@l survive objdump, which prints them as 0.
				if !strings.Contains(lines[index].Assembly, "@toc@") {
					lines[index].Assembly = assembly
				}
				functions[functionName] = lines
				instructionIndexes[functionName] = index + 1
			} else {
				functions[functionName] = append(functions[functionName], internal.Line{
					Assembly: assembly,
					Binary:   binary.String(),
				})
				instructionIndexes[functionName] = len(functions[functionName])
			}
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

func stackScratchSize(lines []internal.Line) int {
	maxOffset := 0
	for _, line := range lines {
		matches := stackRefLine.FindAllStringSubmatch(line.Assembly, -1)
		for _, match := range matches {
			offset := 0
			if _, err := fmt.Sscanf(match[1], "%d", &offset); err == nil && offset > maxOffset {
				maxOffset = offset
			}
		}
	}
	if maxOffset%8 != 0 {
		maxOffset += 8 - maxOffset%8
	}
	return maxOffset
}

func returnBranch(asm string) (string, bool) {
	switch asm {
	case "blr":
		return "BR", true
	case "beqlr", "beqlr 0":
		return "BEQ", true
	case "blelr", "blelr 0":
		return "BLE", true
	default:
		return "", false
	}
}

func asmRegisterName(num string) string {
	return "R" + num
}

func mappedRegisterName(num string, replacement int, hasReplacement bool) string {
	if hasReplacement && num == "30" {
		return fmt.Sprintf("R%d", replacement)
	}
	return asmRegisterName(num)
}

func rewriteStackSpill(asm string, frameSize int, replacement int, hasReplacement bool) (string, bool) {
	match := stackMoveLine.FindStringSubmatch(strings.ToLower(asm))
	if len(match) != 4 {
		return "", false
	}
	offset := 0
	if _, err := fmt.Sscanf(match[3], "%d", &offset); err != nil {
		return "", false
	}
	currentOffset := ppc64LinkageSize + frameSize + offset
	reg := mappedRegisterName(match[2], replacement, hasReplacement)
	switch match[1] {
	case "std":
		return fmt.Sprintf("\tMOVD %s, %d(R1)\n", reg, currentOffset), true
	case "ld":
		return fmt.Sprintf("\tMOVD %d(R1), %s\n", currentOffset, reg), true
	case "stw":
		return fmt.Sprintf("\tMOVW %s, %d(R1)\n", reg, currentOffset), true
	case "lwz":
		return fmt.Sprintf("\tMOVWZ %d(R1), %s\n", currentOffset, reg), true
	default:
		return "", false
	}
}

func usedRegisters(lines []internal.Line) map[int]struct{} {
	registers := make(map[int]struct{})
	for _, line := range lines {
		for _, match := range registerLine.FindAllStringSubmatch(strings.ToLower(line.Assembly), -1) {
			reg := 0
			if _, err := fmt.Sscanf(match[1], "%d", &reg); err == nil {
				registers[reg] = struct{}{}
			}
		}
	}
	return registers
}

// R30 is the fixed g register in Go's ppc64le ABI, so machine code translated
// from clang must not clobber it directly.
func chooseReservedReplacement(lines []internal.Line) (int, bool) {
	used := usedRegisters(lines)
	if _, ok := used[30]; !ok {
		return 0, false
	}
	for reg := 29; reg >= 14; reg-- {
		if _, ok := used[reg]; !ok {
			return reg, true
		}
	}
	return 0, false
}

func patchInstructionWord(line internal.Line, word uint32, assembly string) internal.Line {
	line.Assembly = assembly
	line.Binary = string([]byte{byte(word), byte(word >> 8), byte(word >> 16), byte(word >> 24)})
	return line
}

func rewriteReservedRegister(line internal.Line, replacement int) (internal.Line, bool) {
	asm := strings.ToLower(strings.TrimSpace(line.Assembly))
	if !strings.Contains(asm, "r30") || len(line.Binary) != 4 {
		return line, false
	}
	delta := uint32(replacement - 30)
	word := uint32(line.Binary[0]) | uint32(line.Binary[1])<<8 | uint32(line.Binary[2])<<16 | uint32(line.Binary[3])<<24
	replacementName := fmt.Sprintf("r%d", replacement)
	switch asm {
	case "add r30,r0,r3":
		return patchInstructionWord(line, word+(delta<<21), fmt.Sprintf("add %s,r0,r3", replacementName)), true
	case "addi r0,r30,-4":
		return patchInstructionWord(line, word+(delta<<16), fmt.Sprintf("addi r0,%s,-4", replacementName)), true
	case "mulld r30,r11,r8":
		return patchInstructionWord(line, word+(delta<<21), fmt.Sprintf("mulld %s,r11,r8", replacementName)), true
	case "sldi r30,r30,2":
		return patchInstructionWord(line, word+(delta<<21)+(delta<<16), fmt.Sprintf("sldi %s,%s,2", replacementName, replacementName)), true
	case "add r30,r5,r30":
		return patchInstructionWord(line, word+(delta<<21)+(delta<<11), fmt.Sprintf("add %s,r5,%s", replacementName, replacementName)), true
	case "stfsx f0,r30,r27":
		return patchInstructionWord(line, word+(delta<<16), fmt.Sprintf("stfsx f0,%s,r27", replacementName)), true
	case "lwz r30,0(r12)":
		return patchInstructionWord(line, word+(delta<<21), fmt.Sprintf("lwz %s,0(r12)", replacementName)), true
	case "stw r30,0(r3)":
		return patchInstructionWord(line, word+(delta<<21), fmt.Sprintf("stw %s,0(r3)", replacementName)), true
	default:
		return line, false
	}
}

func rewriteOverflowLoad(line internal.Line, offsetMap map[int]overflowParam, replacement int, hasReplacement bool) (string, bool) {
	match := overflowLoadLine.FindStringSubmatch(strings.ToLower(strings.TrimSpace(line.Assembly)))
	if len(match) != 3 {
		return "", false
	}
	oldOffset := 0
	if _, err := fmt.Sscanf(match[2], "%d", &oldOffset); err != nil {
		return "", false
	}
	overflow, ok := offsetMap[oldOffset]
	if !ok {
		return "", false
	}
	reg := mappedRegisterName(match[1], replacement, hasReplacement)
	switch overflow.param.Type {
	case "_Bool":
		return fmt.Sprintf("\tMOVBZ %s+%d(FP), %s\n", overflow.param.Name, overflow.offset, reg), true
	case "int64_t", "long":
		return fmt.Sprintf("\tMOVD %s+%d(FP), %s\n", overflow.param.Name, overflow.offset, reg), true
	default:
		if overflow.param.Pointer {
			return fmt.Sprintf("\tMOVD %s+%d(FP), %s\n", overflow.param.Name, overflow.offset, reg), true
		}
		return "", false
	}
}

type overflowParam struct {
	offset int
	slot   int
	param  internal.Parameter
}

func tocSymbol(symbol string) string {
	if mapped, ok := dataAnchors[symbol]; ok {
		return mapped
	}
	return symbol
}

func rewriteTOCAddressLoad(lines []internal.Line, index int) (string, bool) {
	if index+1 >= len(lines) {
		return "", false
	}
	high := tocHighLine.FindStringSubmatch(strings.ToLower(strings.TrimSpace(lines[index].Assembly)))
	low := tocLowLine.FindStringSubmatch(strings.ToLower(strings.TrimSpace(lines[index+1].Assembly)))
	if len(high) != 3 || len(low) != 4 {
		return "", false
	}
	if high[1] != low[1] || high[1] != low[2] || high[2] != low[3] {
		return "", false
	}
	return fmt.Sprintf("\tMOVD $%s<>(SB), R%s\n", tocSymbol(high[2]), high[1]), true
}

func generateGoAssembly(buildTags string, header string, goAssemblyPath string, functions []internal.Function) error {
	var builder strings.Builder
	builder.WriteString(buildTags)
	builder.WriteString(header)
	builder.WriteString(internal.GenerateDataSymbols(dataSymbols, binary.LittleEndian))
	for _, function := range functions {
		var body strings.Builder
		var overflowParams []overflowParam
		registerSlot, fpRegisterCount, offset := 0, 0, 0
		for _, param := range function.Parameters {
			sz := parameterSize(param)
			if offset%sz != 0 {
				offset += sz - offset%sz
			}
			if !param.Pointer && (param.Type == "double" || param.Type == "float") {
				if registerSlot < len(registers) && fpRegisterCount < len(fpRegisters) {
					if param.Type == "double" {
						body.WriteString(fmt.Sprintf("\tFMOVD %s+%d(FP), %s\n", param.Name, offset, fpRegisters[fpRegisterCount]))
					} else {
						body.WriteString(fmt.Sprintf("\tFMOVS %s+%d(FP), %s\n", param.Name, offset, fpRegisters[fpRegisterCount]))
					}
					fpRegisterCount++
				} else if registerSlot >= len(registers) {
					overflowParams = append(overflowParams, overflowParam{offset: offset, slot: registerSlot, param: param})
				}
			} else {
				if registerSlot < len(registers) {
					if param.Type == "_Bool" {
						body.WriteString(fmt.Sprintf("\tMOVBZ %s+%d(FP), %s\n", param.Name, offset, registers[registerSlot]))
					} else {
						body.WriteString(fmt.Sprintf("\tMOVD %s+%d(FP), %s\n", param.Name, offset, registers[registerSlot]))
					}
				} else {
					overflowParams = append(overflowParams, overflowParam{offset: offset, slot: registerSlot, param: param})
				}
			}
			registerSlot++
			offset += sz
		}
		if offset%8 != 0 {
			offset += 8 - offset%8
		}
		resultOffset := offset
		argSize := resultOffset + resultSize(function.Type)
		replacement, hasReplacement := chooseReservedReplacement(function.Lines)
		if hasReplacement && replacement == 0 {
			return fmt.Errorf("ppc64le function %s uses r30 but no free callee-saved register is available", function.Name)
		}
		scratchSize := stackScratchSize(function.Lines)
		frameSize := scratchSize
		returnLabel := fmt.Sprintf("%s_return", function.Name)
		overflowOffsetMap := make(map[int]overflowParam)

		builder.WriteString(fmt.Sprintf("\nTEXT ·%v(SB), 4, $%d-%d\n", function.Name, frameSize, argSize))
		builder.WriteString(body.String())
		for _, overflow := range overflowParams {
			originalOffset := 96 + (overflow.slot-len(registers))*8
			overflowOffsetMap[originalOffset] = overflow
		}
		builder.WriteString(fmt.Sprintf("\tMOVD $·%s(SB), R12\n", function.Name))
		for i := 0; i < len(function.Lines); i++ {
			line := function.Lines[i]
			if line.Assembly == "" {
				continue
			}
			if rewritten, ok := rewriteTOCAddressLoad(function.Lines, i); ok {
				builder.WriteString(rewritten)
				i++
			} else if branch, ok := returnBranch(line.Assembly); ok {
				builder.WriteString(fmt.Sprintf("\t%s %s\n", branch, returnLabel))
			} else if rewritten, ok := rewriteOverflowLoad(line, overflowOffsetMap, replacement, hasReplacement); ok {
				builder.WriteString(rewritten)
			} else if rewritten, ok := rewriteStackSpill(line.Assembly, frameSize, replacement, hasReplacement); ok {
				builder.WriteString(rewritten)
			} else if hasReplacement {
				if rewritten, ok := rewriteReservedRegister(line, replacement); ok {
					builder.WriteString(generateLine(rewritten))
				} else if strings.Contains(strings.ToLower(line.Assembly), "r30") {
					return fmt.Errorf("unhandled ppc64le r30 instruction in %s: %s", function.Name, line.Assembly)
				} else {
					builder.WriteString(generateLine(line))
				}
			} else {
				builder.WriteString(generateLine(line))
			}
		}
		builder.WriteString(returnLabel)
		builder.WriteString(":\n")
		if function.Type != "void" {
			switch function.Type {
			case "int64_t", "long":
				builder.WriteString(fmt.Sprintf("\tMOVD R3, result+%d(FP)\n", resultOffset))
			case "_Bool":
				builder.WriteString(fmt.Sprintf("\tMOVB R3, result+%d(FP)\n", resultOffset))
			case "double":
				builder.WriteString(fmt.Sprintf("\tFMOVD F1, result+%d(FP)\n", resultOffset))
			case "float":
				builder.WriteString(fmt.Sprintf("\tFMOVS F1, result+%d(FP)\n", resultOffset))
			default:
				return fmt.Errorf("unsupported return type: %v", function.Type)
			}
		}
		builder.WriteString("\tRET\n")
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
