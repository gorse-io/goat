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
	"strconv"
	"strings"
	"unicode"

	"github.com/klauspost/asmfmt"
	"github.com/samber/lo"
)

const (
	buildTags   = "//go:build !noasm && arm64\n"
	buildTarget = "arm64-linux-gnu"
)

var (
	attributeLine = regexp.MustCompile(`^\s+\..+$`)
	nameLine      = regexp.MustCompile(`^\w+:.+$`)
	labelLine     = regexp.MustCompile(`^\.\w+_\d+:.*$`)
	codeLine      = regexp.MustCompile(`^\s+\w+.+$`)
	jmpLine       = regexp.MustCompile(`^(b|b\.\w{2})\t\.\w+_\d+$`)

	symbolLine = regexp.MustCompile(`^\w+\s+<\w+>:$`)
	dataLine   = regexp.MustCompile(`^\w+:\s+\w+\s+.+$`)

	registers   = []string{"R0", "R1", "R2", "R3", "R4", "R5", "R6", "R7"}
	fpRegisters = []string{"F0", "F1", "F2", "F3", "F4", "F5", "F6", "F7"}
	// NEON 128-bit vector registers (V0-V7 for parameter passing per ARM64 ABI)
	neonRegisters = []string{"V0", "V1", "V2", "V3", "V4", "V5", "V6", "V7"}
)

type Line struct {
	Labels   []string
	Assembly string
	Binary   string
}

func (line *Line) String() string {
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
	} else {
		builder.WriteString("\t")
		builder.WriteString(fmt.Sprintf("WORD $0x%v", line.Binary))
		builder.WriteString("\t// ")
		builder.WriteString(line.Assembly)
		builder.WriteString("\n")
	}
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
			if offset, err := strconv.Atoi(m[1]); err == nil && offset > stackSizes[functionName] {
				stackSizes[functionName] = offset
			}
			continue
		}
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
			if sz := NeonTypeSize(function.Type); sz > 0 {
				returnSize = sz // Use actual NEON type size
			} else if sz, ok := supportedTypes[function.Type]; ok {
				returnSize = sz // Use actual scalar type size
			} else {
				returnSize = 8 // Default 8-byte slot for pointers/unknown types
			}
		}
		registerCount, fpRegisterCount, neonRegisterCount, offset := 0, 0, 0, 0
		var stack []lo.Tuple2[int, Parameter]
		var argsBuilder strings.Builder
		for _, param := range function.Parameters {
			// Calculate slot size based on type
			sz := 8 // Default 8-byte slot
			alignTo := 8
			if !param.Pointer {
				if neonSz := NeonTypeSize(param.Type); neonSz > 0 {
					sz = neonSz // Use actual NEON type size
					alignTo = sz
					if alignTo > 16 {
						alignTo = 16 // Cap alignment at 16 bytes
					}
				} else if param.Type == "float" {
					sz = 4       // float32 is 4 bytes
					alignTo = 4  // 4-byte alignment
				}
				// double, int64_t, long, pointers use default 8 bytes
			}
			// Align offset
			if offset%alignTo != 0 {
				offset += alignTo - offset%alignTo
			}
			if !param.Pointer && IsNeonType(param.Type) {
				// NEON vector type - load into V register(s)
				vecCount := NeonVectorCount(param.Type)
				is64bit := IsNeon64Type(param.Type)

				if neonRegisterCount+vecCount <= len(neonRegisters) {
					for v := 0; v < vecCount; v++ {
						vecOffset := offset + v*16
						if is64bit {
							vecOffset = offset + v*8
						}

						if is64bit {
							// 64-bit vector: single MOVD, load into D[0] only
							argsBuilder.WriteString(fmt.Sprintf("\tMOVD %s+%d(FP), R9\n", param.Name, vecOffset))
							argsBuilder.WriteString(fmt.Sprintf("\tVMOV R9, %s.D[0]\n", neonRegisters[neonRegisterCount+v]))
						} else {
							// 128-bit vector: two MOVDs, load into D[0] and D[1]
							// Use _N suffixes (N=offset within param) so go vet accepts different offsets
							localOffset := v * 16 // offset within this parameter's storage
							argsBuilder.WriteString(fmt.Sprintf("\tMOVD %s_%d+%d(FP), R9\n", param.Name, localOffset, vecOffset))
							argsBuilder.WriteString(fmt.Sprintf("\tMOVD %s_%d+%d(FP), R10\n", param.Name, localOffset+8, vecOffset+8))
							argsBuilder.WriteString(fmt.Sprintf("\tVMOV R9, %s.D[0]\n", neonRegisters[neonRegisterCount+v]))
							argsBuilder.WriteString(fmt.Sprintf("\tVMOV R10, %s.D[1]\n", neonRegisters[neonRegisterCount+v]))
						}
					}
					neonRegisterCount += vecCount
				} else {
					stack = append(stack, lo.Tuple2[int, Parameter]{A: offset, B: param})
				}
			} else if !param.Pointer && (param.Type == "float" || param.Type == "double") {
				if fpRegisterCount < len(fpRegisters) {
					if param.Type == "float" {
						argsBuilder.WriteString(fmt.Sprintf("\tFMOVS %s+%d(FP), %s\n", param.Name, offset, fpRegisters[fpRegisterCount]))
					} else {
						argsBuilder.WriteString(fmt.Sprintf("\tFMOVD %s+%d(FP), %s\n", param.Name, offset, fpRegisters[fpRegisterCount]))
					}
					fpRegisterCount++
				} else {
					stack = append(stack, lo.Tuple2[int, Parameter]{A: offset, B: param})
				}
			} else {
				if registerCount < len(registers) {
					argsBuilder.WriteString(fmt.Sprintf("\tMOVD %s+%d(FP), %s\n", param.Name, offset, registers[registerCount]))
					registerCount++
				} else {
					stack = append(stack, lo.Tuple2[int, Parameter]{A: offset, B: param})
				}
			}
			offset += sz
		}
		// Check if 128-bit+ NEON types are used (for stack frame alignment)
		has128BitNeon := false
		for _, param := range function.Parameters {
			if !param.Pointer && IsNeonType(param.Type) && !IsNeon64Type(param.Type) {
				has128BitNeon = true
				break
			}
		}
		if !has128BitNeon && IsNeonType(function.Type) && !IsNeon64Type(function.Type) {
			has128BitNeon = true
		}
		// Note: Don't align offset to 16 bytes here - Go's ABI only requires 8-byte
		// alignment for return values, which is handled below
		stackOffset := 0
		if len(stack) > 0 {
			for i := 0; i < len(stack); i++ {
				if neonSz := NeonTypeSize(stack[i].B.Type); neonSz > 0 {
					// NEON vector: copy all bytes to stack
					is64bit := IsNeon64Type(stack[i].B.Type)
					vecCount := NeonVectorCount(stack[i].B.Type)
					for v := 0; v < vecCount; v++ {
						srcOffset := stack[i].A + v*(16)
						if is64bit {
							srcOffset = stack[i].A + v*8
						}
						if is64bit {
							// 64-bit vector: single 8-byte copy
							argsBuilder.WriteString(fmt.Sprintf("\tMOVD %s+%d(FP), R8\n", stack[i].B.Name, srcOffset))
							argsBuilder.WriteString(fmt.Sprintf("\tMOVD R8, %d(RSP)\n", stackOffset))
							stackOffset += 8
						} else {
							// 128-bit vector: two 8-byte copies
							// Use _N suffixes (N=offset within param) so go vet accepts different offsets
							localOffset := v * 16 // offset within this parameter's storage
							argsBuilder.WriteString(fmt.Sprintf("\tMOVD %s_%d+%d(FP), R8\n", stack[i].B.Name, localOffset, srcOffset))
							argsBuilder.WriteString(fmt.Sprintf("\tMOVD R8, %d(RSP)\n", stackOffset))
							argsBuilder.WriteString(fmt.Sprintf("\tMOVD %s_%d+%d(FP), R8\n", stack[i].B.Name, localOffset+8, srcOffset+8))
							argsBuilder.WriteString(fmt.Sprintf("\tMOVD R8, %d(RSP)\n", stackOffset+8))
							stackOffset += 16
						}
					}
				} else {
					argsBuilder.WriteString(fmt.Sprintf("\tMOVD %s+%d(FP), R8\n", stack[i].B.Name, stack[i].A))
					argsBuilder.WriteString(fmt.Sprintf("\tMOVD R8, %d(RSP)\n", stackOffset))
					if stack[i].B.Pointer {
						stackOffset += 8
					} else {
						stackOffset += supportedTypes[stack[i].B.Type]
					}
				}
			}
		}
		// Align to 16 bytes only if 128-bit+ NEON types are used
		if has128BitNeon && stackOffset%16 != 0 {
			stackOffset += 16 - stackOffset%16
		}
		// Return value must be 8-byte aligned in Go's ABI
		if offset%8 != 0 {
			offset += 8 - offset%8
		}
		frameSize := max(stackOffset, function.StackSize)
		// Go's assembler requires frame sizes to be aligned to 16 bytes on arm64
		if frameSize%16 != 0 {
			frameSize += 16 - frameSize%16
		}
		builder.WriteString(fmt.Sprintf("\nTEXT ·%v(SB), $%d-%d\n",
			function.Name, frameSize, offset+returnSize))
		builder.WriteString(argsBuilder.String())
		for _, line := range function.Lines {
			for _, label := range line.Labels {
				builder.WriteString(label)
				builder.WriteString(":\n")
			}
			if line.Assembly == "ret" {
				if function.Type != "void" {
					switch function.Type {
					case "int64_t", "long", "_Bool":
						builder.WriteString(fmt.Sprintf("\tMOVD R0, result+%d(FP)\n", offset))
					case "double":
						builder.WriteString(fmt.Sprintf("\tFMOVD F0, result+%d(FP)\n", offset))
					case "float":
						builder.WriteString(fmt.Sprintf("\tFMOVS F0, result+%d(FP)\n", offset))
					default:
						// Check for NEON vector return types
						if IsNeonType(function.Type) {
							is64bit := IsNeon64Type(function.Type)
							vecCount := NeonVectorCount(function.Type)
							resultOffset := offset
							for v := 0; v < vecCount; v++ {
								vReg := neonRegisters[v] // V0, V1, V2, V3...
								if is64bit {
									// 64-bit vector: extract D[0] only
									builder.WriteString(fmt.Sprintf("\tVMOV %s.D[0], R9\n", vReg))
									builder.WriteString(fmt.Sprintf("\tMOVD R9, result+%d(FP)\n", resultOffset))
									resultOffset += 8
								} else {
									// 128-bit vector: extract both D[0] and D[1]
									// Use _N suffixes (N=offset within result) so go vet accepts different offsets
									localOffset := v * 16 // offset within result parameter
									builder.WriteString(fmt.Sprintf("\tVMOV %s.D[0], R9\n", vReg))
									builder.WriteString(fmt.Sprintf("\tVMOV %s.D[1], R10\n", vReg))
									builder.WriteString(fmt.Sprintf("\tMOVD R9, result_%d+%d(FP)\n", localOffset, resultOffset))
									builder.WriteString(fmt.Sprintf("\tMOVD R10, result_%d+%d(FP)\n", localOffset+8, resultOffset+8))
									resultOffset += 16
								}
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
