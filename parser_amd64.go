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
	"runtime"
	"strconv"
	"strings"
	"unicode"

	"github.com/klauspost/asmfmt"
	"github.com/samber/lo"
)

var (
	buildTags   = "//go:build !noasm && amd64\n"
	buildTarget = func() string {
		if runtime.GOOS == "darwin" {
			return "x86_64-apple-darwin"
		}
		return "x86_64-linux-gnu"
	}()
)

var (
	attributeLine = regexp.MustCompile(`^\s+\..+$`)
	nameLine      = regexp.MustCompile(`^\w+:.+$`)
	labelLine     = regexp.MustCompile(`^\.\w+_\d+:.*$`)
	codeLine      = regexp.MustCompile(`^\s+\w+.+$`)

	symbolLine = regexp.MustCompile(`^\w+\s+<\w+>:$`)
	dataLine   = regexp.MustCompile(`^\w+:\s+\w+\s+.+$`)

	registers    = []string{"DI", "SI", "DX", "CX", "R8", "R9"}
	xmmRegisters = []string{"X0", "X1", "X2", "X3", "X4", "X5", "X6", "X7"} // 128-bit SSE
	ymmRegisters = []string{"Y0", "Y1", "Y2", "Y3", "Y4", "Y5", "Y6", "Y7"} // 256-bit AVX
	zmmRegisters = []string{"Z0", "Z1", "Z2", "Z3", "Z4", "Z5", "Z6", "Z7"} // 512-bit AVX-512
)

type Line struct {
	Labels   []string
	Assembly string
	Binary   []string
}

func (line *Line) String() string {
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
		cfiRegex     = regexp.MustCompile(`^\s+\.cfi_def_cfa_offset\s+(\d+)`)
	)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if m := cfiRegex.FindStringSubmatch(line); m != nil {
			if offset, err := strconv.Atoi(m[1]); err == nil {
				// On x86-64, CFA offset includes the 8-byte return address
				// pushed by the call instruction. Subtract it to get the
				// actual stack frame usage.
				adjusted := max(0, offset-8)
				if adjusted > stackSizes[functionName] {
					stackSizes[functionName] = adjusted
				}
			}
			continue
		}
		if attributeLine.MatchString(line) {
			continue
		} else if nameLine.MatchString(line) {
			functionName = strings.Split(line, ":")[0]
			// On macOS, function names are prefixed with underscore - strip it
			if runtime.GOOS == "darwin" && strings.HasPrefix(functionName, "_") {
				functionName = functionName[1:]
			}
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
			// On macOS, function names are prefixed with underscore - strip it
			if runtime.GOOS == "darwin" && strings.HasPrefix(functionName, "_") {
				functionName = functionName[1:]
			}
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
		// Calculate return size based on type
		returnSize := 0
		if function.Type != "void" {
			if sz := X86SIMDTypeSize(function.Type); sz > 0 {
				returnSize = sz // Use actual SIMD type size
			} else if sz, ok := supportedTypes[function.Type]; ok {
				returnSize = sz // Use actual scalar type size
			} else {
				returnSize = 8 // Default 8-byte slot for pointers/unknown types
			}
		}

		registerIndex, xmmRegisterIndex, offset := 0, 0, 0
		var stack []lo.Tuple2[int, Parameter]
		var argsBuilder strings.Builder

		for _, param := range function.Parameters {
			// Calculate slot size based on type
			sz := 8 // Default 8-byte slot for scalars in Go ABI0
			if !param.Pointer {
				if simdSz := X86SIMDTypeSize(param.Type); simdSz > 0 {
					sz = simdSz // Use actual SIMD type size
				}
			}

			// Align offset to slot size (max 16 for Go ABI0)
			alignTo := sz
			if alignTo > 16 {
				alignTo = 16 // Cap alignment at 16 bytes for Go stack
			}
			if offset%alignTo != 0 {
				offset += alignTo - offset%alignTo
			}

			if !param.Pointer && IsX86SIMDType(param.Type) {
				// x86 SIMD vector type - load into XMM/YMM/ZMM register
				if xmmRegisterIndex < len(xmmRegisters) {
					switch {
					case X86SIMDTypeSize(param.Type) == 64:
						// AVX-512 (512-bit): load into ZMM via 8 MOVQs
						// Use _N suffixes (N=offset within param) so go vet accepts different offsets
						argsBuilder.WriteString(fmt.Sprintf("\tMOVQ %s_0+%d(FP), AX\n", param.Name, offset))
						argsBuilder.WriteString(fmt.Sprintf("\tMOVQ %s_8+%d(FP), BX\n", param.Name, offset+8))
						argsBuilder.WriteString(fmt.Sprintf("\tMOVQ AX, X14\n")) // temp
						argsBuilder.WriteString(fmt.Sprintf("\tPINSRQ $1, BX, X14\n"))
						argsBuilder.WriteString(fmt.Sprintf("\tMOVQ %s_16+%d(FP), AX\n", param.Name, offset+16))
						argsBuilder.WriteString(fmt.Sprintf("\tMOVQ %s_24+%d(FP), BX\n", param.Name, offset+24))
						argsBuilder.WriteString(fmt.Sprintf("\tMOVQ AX, X15\n"))
						argsBuilder.WriteString(fmt.Sprintf("\tPINSRQ $1, BX, X15\n"))
						argsBuilder.WriteString(fmt.Sprintf("\tVINSERTF128 $1, X15, Y14, Y14\n"))
						argsBuilder.WriteString(fmt.Sprintf("\tMOVQ %s_32+%d(FP), AX\n", param.Name, offset+32))
						argsBuilder.WriteString(fmt.Sprintf("\tMOVQ %s_40+%d(FP), BX\n", param.Name, offset+40))
						argsBuilder.WriteString(fmt.Sprintf("\tMOVQ AX, X15\n"))
						argsBuilder.WriteString(fmt.Sprintf("\tPINSRQ $1, BX, X15\n"))
						argsBuilder.WriteString(fmt.Sprintf("\tMOVQ %s_48+%d(FP), AX\n", param.Name, offset+48))
						argsBuilder.WriteString(fmt.Sprintf("\tMOVQ %s_56+%d(FP), BX\n", param.Name, offset+56))
						argsBuilder.WriteString(fmt.Sprintf("\tMOVQ AX, X13\n"))
						argsBuilder.WriteString(fmt.Sprintf("\tPINSRQ $1, BX, X13\n"))
						argsBuilder.WriteString(fmt.Sprintf("\tVINSERTF128 $1, X13, Y15, Y15\n"))
						argsBuilder.WriteString(fmt.Sprintf("\tVINSERTF64X4 $1, Y15, Z14, %s\n", zmmRegisters[xmmRegisterIndex]))
					case X86SIMDTypeSize(param.Type) == 32:
						// AVX (256-bit): load into YMM via 4 MOVQs + VINSERTF128
						// Use _N suffixes (N=offset within param) so go vet accepts different offsets
						argsBuilder.WriteString(fmt.Sprintf("\tMOVQ %s_0+%d(FP), AX\n", param.Name, offset))
						argsBuilder.WriteString(fmt.Sprintf("\tMOVQ %s_8+%d(FP), BX\n", param.Name, offset+8))
						argsBuilder.WriteString(fmt.Sprintf("\tMOVQ AX, X14\n"))
						argsBuilder.WriteString(fmt.Sprintf("\tPINSRQ $1, BX, X14\n"))
						argsBuilder.WriteString(fmt.Sprintf("\tMOVQ %s_16+%d(FP), AX\n", param.Name, offset+16))
						argsBuilder.WriteString(fmt.Sprintf("\tMOVQ %s_24+%d(FP), BX\n", param.Name, offset+24))
						argsBuilder.WriteString(fmt.Sprintf("\tMOVQ AX, X15\n"))
						argsBuilder.WriteString(fmt.Sprintf("\tPINSRQ $1, BX, X15\n"))
						argsBuilder.WriteString(fmt.Sprintf("\tVINSERTF128 $1, X15, Y14, %s\n", ymmRegisters[xmmRegisterIndex]))
					default:
						// SSE (128-bit): load into XMM via 2 MOVQs + PINSRQ
						// Use _N suffixes (N=offset within param) so go vet accepts different offsets
						argsBuilder.WriteString(fmt.Sprintf("\tMOVQ %s_0+%d(FP), AX\n", param.Name, offset))
						argsBuilder.WriteString(fmt.Sprintf("\tMOVQ %s_8+%d(FP), BX\n", param.Name, offset+8))
						argsBuilder.WriteString(fmt.Sprintf("\tMOVQ AX, %s\n", xmmRegisters[xmmRegisterIndex]))
						argsBuilder.WriteString(fmt.Sprintf("\tPINSRQ $1, BX, %s\n", xmmRegisters[xmmRegisterIndex]))
					}
					xmmRegisterIndex++
				} else {
					stack = append(stack, lo.Tuple2[int, Parameter]{A: offset, B: param})
				}
			} else if !param.Pointer && (param.Type == "double" || param.Type == "float") {
				if xmmRegisterIndex < len(xmmRegisters) {
					if param.Type == "double" {
						argsBuilder.WriteString(fmt.Sprintf("\tMOVSD %s+%d(FP), %s\n", param.Name, offset, xmmRegisters[xmmRegisterIndex]))
					} else {
						argsBuilder.WriteString(fmt.Sprintf("\tMOVSS %s+%d(FP), %s\n", param.Name, offset, xmmRegisters[xmmRegisterIndex]))
					}
					xmmRegisterIndex++
				} else {
					stack = append(stack, lo.Tuple2[int, Parameter]{A: offset, B: param})
				}
			} else {
				if registerIndex < len(registers) {
					argsBuilder.WriteString(fmt.Sprintf("\tMOVQ %s+%d(FP), %s\n", param.Name, offset, registers[registerIndex]))
					registerIndex++
				} else {
					stack = append(stack, lo.Tuple2[int, Parameter]{A: offset, B: param})
				}
			}
			offset += sz
		}

		// Check if SIMD types are used (for stack frame alignment)
		hasSIMD := false
		for _, param := range function.Parameters {
			if !param.Pointer && IsX86SIMDType(param.Type) {
				hasSIMD = true
				break
			}
		}
		if !hasSIMD && IsX86SIMDType(function.Type) {
			hasSIMD = true
		}
		// Note: Don't align offset to 16 bytes here - Go's ABI only requires 8-byte
		// alignment for return values

		// Calculate stack frame size (for spilled parameters)
		stackOffset := 0
		if len(stack) > 0 {
			for i := 0; i < len(stack); i++ {
				if simdSz := X86SIMDTypeSize(stack[i].B.Type); simdSz > 0 {
					stackOffset += simdSz
				} else if stack[i].B.Pointer {
					stackOffset += 8
				} else {
					stackOffset += supportedTypes[stack[i].B.Type]
				}
			}
		}
		// Align stack frame
		if hasSIMD && stackOffset%16 != 0 {
			stackOffset += 16 - stackOffset%16
		}

		frameSize := max(stackOffset, function.StackSize)
		// Go's assembler requires frame sizes to be aligned to 16 bytes
		if frameSize%16 != 0 {
			frameSize += 16 - frameSize%16
		}
		builder.WriteString(fmt.Sprintf("\nTEXT ·%v(SB), $%d-%d\n",
			function.Name, frameSize, offset+returnSize))
		builder.WriteString(argsBuilder.String())

		// Push stack parameters if needed
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
						// Check for x86 SIMD vector return types
						if IsX86SIMDType(function.Type) {
							resultOffset := offset
							switch X86SIMDTypeSize(function.Type) {
							case 64:
								// AVX-512 (512-bit): extract from ZMM0 via stores
								// Use _N suffixes (N=offset within result) so go vet accepts different offsets
								builder.WriteString("\tVEXTRACTF64X4 $0, Z0, Y14\n")
								builder.WriteString("\tVEXTRACTF64X4 $1, Z0, Y15\n")
								builder.WriteString("\tVEXTRACTF128 $0, Y14, X14\n")
								builder.WriteString("\tMOVQ X14, AX\n")
								builder.WriteString("\tPEXTRQ $1, X14, BX\n")
								builder.WriteString(fmt.Sprintf("\tMOVQ AX, result_0+%d(FP)\n", resultOffset))
								builder.WriteString(fmt.Sprintf("\tMOVQ BX, result_8+%d(FP)\n", resultOffset+8))
								builder.WriteString("\tVEXTRACTF128 $1, Y14, X14\n")
								builder.WriteString("\tMOVQ X14, AX\n")
								builder.WriteString("\tPEXTRQ $1, X14, BX\n")
								builder.WriteString(fmt.Sprintf("\tMOVQ AX, result_16+%d(FP)\n", resultOffset+16))
								builder.WriteString(fmt.Sprintf("\tMOVQ BX, result_24+%d(FP)\n", resultOffset+24))
								builder.WriteString("\tVEXTRACTF128 $0, Y15, X15\n")
								builder.WriteString("\tMOVQ X15, AX\n")
								builder.WriteString("\tPEXTRQ $1, X15, BX\n")
								builder.WriteString(fmt.Sprintf("\tMOVQ AX, result_32+%d(FP)\n", resultOffset+32))
								builder.WriteString(fmt.Sprintf("\tMOVQ BX, result_40+%d(FP)\n", resultOffset+40))
								builder.WriteString("\tVEXTRACTF128 $1, Y15, X15\n")
								builder.WriteString("\tMOVQ X15, AX\n")
								builder.WriteString("\tPEXTRQ $1, X15, BX\n")
								builder.WriteString(fmt.Sprintf("\tMOVQ AX, result_48+%d(FP)\n", resultOffset+48))
								builder.WriteString(fmt.Sprintf("\tMOVQ BX, result_56+%d(FP)\n", resultOffset+56))
							case 32:
								// AVX (256-bit): extract from YMM0 via VEXTRACTF128 + MOVQ/PEXTRQ
								// Use _N suffixes (N=offset within result) so go vet accepts different offsets
								builder.WriteString("\tVEXTRACTF128 $0, Y0, X14\n")
								builder.WriteString("\tMOVQ X14, AX\n")
								builder.WriteString("\tPEXTRQ $1, X14, BX\n")
								builder.WriteString(fmt.Sprintf("\tMOVQ AX, result_0+%d(FP)\n", resultOffset))
								builder.WriteString(fmt.Sprintf("\tMOVQ BX, result_8+%d(FP)\n", resultOffset+8))
								builder.WriteString("\tVEXTRACTF128 $1, Y0, X14\n")
								builder.WriteString("\tMOVQ X14, AX\n")
								builder.WriteString("\tPEXTRQ $1, X14, BX\n")
								builder.WriteString(fmt.Sprintf("\tMOVQ AX, result_16+%d(FP)\n", resultOffset+16))
								builder.WriteString(fmt.Sprintf("\tMOVQ BX, result_24+%d(FP)\n", resultOffset+24))
							case 16:
								// SSE (128-bit): extract from XMM0 via MOVQ + PEXTRQ
								// Use _N suffixes (N=offset within result) so go vet accepts different offsets
								builder.WriteString("\tMOVQ X0, AX\n")
								builder.WriteString("\tPEXTRQ $1, X0, BX\n")
								builder.WriteString(fmt.Sprintf("\tMOVQ AX, result_0+%d(FP)\n", resultOffset))
								builder.WriteString(fmt.Sprintf("\tMOVQ BX, result_8+%d(FP)\n", resultOffset+8))
							}
						} else {
							return fmt.Errorf("unsupported return type: %v", function.Type)
						}
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
