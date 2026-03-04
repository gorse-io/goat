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
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/sys/cpu"
	"modernc.org/cc/v4"
)

var supportedTypes = map[string]int{
	"int64_t": 8,
	"long":    8,
	"float":   4,
	"double":  8,
	"_Bool":   1,
}

// NEON vector types (128-bit / 16 bytes)
var neon128Types = map[string]int{
	// Integer vectors
	"int8x16_t":  16,
	"int16x8_t":  16,
	"int32x4_t":  16,
	"int64x2_t":  16,
	"uint8x16_t": 16,
	"uint16x8_t": 16,
	"uint32x4_t": 16,
	"uint64x2_t": 16,
	// Float vectors
	"float32x4_t": 16,
	"float64x2_t": 16,
	// Half-precision (float16)
	"float16x8_t": 16,
	// BFloat16
	"bfloat16x8_t": 16,
	// Polynomial types
	"poly8x16_t": 16,
	"poly16x8_t": 16,
	"poly64x2_t": 16,
	"poly128_t":  16,
}

// NEON vector types (64-bit / 8 bytes)
var neon64Types = map[string]int{
	// Integer vectors
	"int8x8_t":   8,
	"int16x4_t":  8,
	"int32x2_t":  8,
	"int64x1_t":  8,
	"uint8x8_t":  8,
	"uint16x4_t": 8,
	"uint32x2_t": 8,
	"uint64x1_t": 8,
	// Float vectors
	"float32x2_t": 8,
	"float64x1_t": 8,
	// Half-precision (float16)
	"float16x4_t": 8,
	// BFloat16
	"bfloat16x4_t": 8,
	// Polynomial types
	"poly8x8_t":  8,
	"poly16x4_t": 8,
	"poly64x1_t": 8,
}

// NEON array types (multiple 128-bit vectors)
var neonArrayTypes = map[string]int{
	// 2 vectors (32 bytes)
	"int8x16x2_t":    32,
	"int16x8x2_t":    32,
	"int32x4x2_t":    32,
	"int64x2x2_t":    32,
	"uint8x16x2_t":   32,
	"uint16x8x2_t":   32,
	"uint32x4x2_t":   32,
	"uint64x2x2_t":   32,
	"float32x4x2_t":  32,
	"float64x2x2_t":  32,
	"float16x8x2_t":  32,
	"bfloat16x8x2_t": 32,
	"poly8x16x2_t":   32,
	"poly16x8x2_t":   32,
	"poly64x2x2_t":   32,
	// 3 vectors (48 bytes)
	"int8x16x3_t":    48,
	"int16x8x3_t":    48,
	"int32x4x3_t":    48,
	"int64x2x3_t":    48,
	"uint8x16x3_t":   48,
	"uint16x8x3_t":   48,
	"uint32x4x3_t":   48,
	"uint64x2x3_t":   48,
	"float32x4x3_t":  48,
	"float64x2x3_t":  48,
	"float16x8x3_t":  48,
	"bfloat16x8x3_t": 48,
	"poly8x16x3_t":   48,
	"poly16x8x3_t":   48,
	"poly64x2x3_t":   48,
	// 4 vectors (64 bytes)
	"int8x16x4_t":    64,
	"int16x8x4_t":    64,
	"int32x4x4_t":    64,
	"int64x2x4_t":    64,
	"uint8x16x4_t":   64,
	"uint16x8x4_t":   64,
	"uint32x4x4_t":   64,
	"uint64x2x4_t":   64,
	"float32x4x4_t":  64,
	"float64x2x4_t":  64,
	"float16x8x4_t":  64,
	"bfloat16x8x4_t": 64,
	"poly8x16x4_t":   64,
	"poly16x8x4_t":   64,
	"poly64x2x4_t":   64,
}

// x86 SSE types (128-bit / 16 bytes)
var sse128Types = map[string]int{
	"__m128":  16, // 4x float32
	"__m128d": 16, // 2x float64
	"__m128i": 16, // various integer types
}

// x86 AVX types (256-bit / 32 bytes)
var avx256Types = map[string]int{
	"__m256":  32, // 8x float32
	"__m256d": 32, // 4x float64
	"__m256i": 32, // various integer types
}

// x86 AVX-512 types (512-bit / 64 bytes)
var avx512Types = map[string]int{
	"__m512":  64, // 16x float32
	"__m512d": 64, // 8x float64
	"__m512i": 64, // various integer types
}

// NEON 64-bit array types (multiple 64-bit vectors)
var neon64ArrayTypes = map[string]int{
	// 2 vectors (16 bytes)
	"int8x8x2_t":     16,
	"int16x4x2_t":    16,
	"int32x2x2_t":    16,
	"int64x1x2_t":    16,
	"uint8x8x2_t":    16,
	"uint16x4x2_t":   16,
	"uint32x2x2_t":   16,
	"uint64x1x2_t":   16,
	"float32x2x2_t":  16,
	"float64x1x2_t":  16,
	"float16x4x2_t":  16,
	"bfloat16x4x2_t": 16,
	"poly8x8x2_t":    16,
	"poly16x4x2_t":   16,
	"poly64x1x2_t":   16,
	// 3 vectors (24 bytes)
	"int8x8x3_t":     24,
	"int16x4x3_t":    24,
	"int32x2x3_t":    24,
	"int64x1x3_t":    24,
	"uint8x8x3_t":    24,
	"uint16x4x3_t":   24,
	"uint32x2x3_t":   24,
	"uint64x1x3_t":   24,
	"float32x2x3_t":  24,
	"float64x1x3_t":  24,
	"float16x4x3_t":  24,
	"bfloat16x4x3_t": 24,
	"poly8x8x3_t":    24,
	"poly16x4x3_t":   24,
	"poly64x1x3_t":   24,
	// 4 vectors (32 bytes)
	"int8x8x4_t":     32,
	"int16x4x4_t":    32,
	"int32x2x4_t":    32,
	"int64x1x4_t":    32,
	"uint8x8x4_t":    32,
	"uint16x4x4_t":   32,
	"uint32x2x4_t":   32,
	"uint64x1x4_t":   32,
	"float32x2x4_t":  32,
	"float64x1x4_t":  32,
	"float16x4x4_t":  32,
	"bfloat16x4x4_t": 32,
	"poly8x8x4_t":    32,
	"poly16x4x4_t":   32,
	"poly64x1x4_t":   32,
}

// IsNeonType returns true if the type is any NEON vector type
func IsNeonType(t string) bool {
	if _, ok := neon128Types[t]; ok {
		return true
	}
	if _, ok := neon64Types[t]; ok {
		return true
	}
	if _, ok := neonArrayTypes[t]; ok {
		return true
	}
	if _, ok := neon64ArrayTypes[t]; ok {
		return true
	}
	return false
}

// NeonTypeSize returns the size in bytes for a NEON type, or 0 if not a NEON type
func NeonTypeSize(t string) int {
	if sz, ok := neon128Types[t]; ok {
		return sz
	}
	if sz, ok := neon64Types[t]; ok {
		return sz
	}
	if sz, ok := neonArrayTypes[t]; ok {
		return sz
	}
	if sz, ok := neon64ArrayTypes[t]; ok {
		return sz
	}
	return 0
}

// NeonVectorCount returns the number of vectors in a NEON type (1 for single, 2-4 for arrays)
func NeonVectorCount(t string) int {
	if _, ok := neon128Types[t]; ok {
		return 1
	}
	if _, ok := neon64Types[t]; ok {
		return 1
	}
	// Check array types by suffix
	if strings.HasSuffix(t, "x2_t") {
		return 2
	}
	if strings.HasSuffix(t, "x3_t") {
		return 3
	}
	if strings.HasSuffix(t, "x4_t") {
		return 4
	}
	return 0
}

// IsNeon64Type returns true if this is a 64-bit (not 128-bit) NEON base type
func IsNeon64Type(t string) bool {
	if _, ok := neon64Types[t]; ok {
		return true
	}
	if _, ok := neon64ArrayTypes[t]; ok {
		return true
	}
	return false
}

// IsX86SIMDType returns true if the type is any x86 SIMD vector type
func IsX86SIMDType(t string) bool {
	if _, ok := sse128Types[t]; ok {
		return true
	}
	if _, ok := avx256Types[t]; ok {
		return true
	}
	if _, ok := avx512Types[t]; ok {
		return true
	}
	return false
}

// X86SIMDTypeSize returns the size in bytes for an x86 SIMD type, or 0 if not an x86 SIMD type
func X86SIMDTypeSize(t string) int {
	if sz, ok := sse128Types[t]; ok {
		return sz
	}
	if sz, ok := avx256Types[t]; ok {
		return sz
	}
	if sz, ok := avx512Types[t]; ok {
		return sz
	}
	return 0
}

// X86SIMDAlignment returns the required alignment for an x86 SIMD type
func X86SIMDAlignment(t string) int {
	if _, ok := sse128Types[t]; ok {
		return 16
	}
	if _, ok := avx256Types[t]; ok {
		return 32
	}
	if _, ok := avx512Types[t]; ok {
		return 64
	}
	return 0
}

type TranslateUnit struct {
	Source     string
	Assembly   string
	Object     string
	GoAssembly string
	Go         string
	Package    string
	Options    []string
	Offset     int
}

func NewTranslateUnit(source string, outputDir string, options ...string) TranslateUnit {
	sourceExt := filepath.Ext(source)
	noExtSourcePath := source[:len(source)-len(sourceExt)]
	noExtSourceBase := filepath.Base(noExtSourcePath)
	return TranslateUnit{
		Source:     source,
		Assembly:   noExtSourcePath + ".s",
		Object:     noExtSourcePath + ".o",
		GoAssembly: filepath.Join(outputDir, noExtSourceBase+".s"),
		Go:         filepath.Join(outputDir, noExtSourceBase+".go"),
		Package:    filepath.Base(outputDir),
		Options:    options,
	}
}

// parseSource parse C source file and extract functions declarations.
func (t *TranslateUnit) parseSource() ([]Function, error) {
	f, err := os.Open(t.Source)
	if err != nil {
		return nil, err
	}
	cfg, err := cc.NewConfig(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return nil, err
	}
	var prologue strings.Builder
	if cpu.RISCV64.HasV {
		prologue.WriteString("#define __riscv_vector 1\n")
		for _, typeStr := range []string{"int64", "uint64", "int32", "uint32", "int16", "uint16", "int8", "uint8", "float64", "float32", "float16"} {
			for i := 1; i <= 8; i *= 2 {
				prologue.WriteString(fmt.Sprintf("typedef char v%sm%d_t;\n", typeStr, i))
			}
		}
	}
	// Add definitions for ARM64 to help parser handle arm_neon.h
	if runtime.GOARCH == "arm64" {
		// Define __bf16 for arm_bf16.h (compiler built-in type)
		prologue.WriteString("typedef short __bf16;\n")
		// Define __fp16 for arm_fp16.h
		prologue.WriteString("typedef short __fp16;\n")
	}
	// Add definitions for AMD64 to help parser handle x86 intrinsics
	if runtime.GOARCH == "amd64" {
		// Define GOAT_PARSER to skip includes during parsing
		// The C file should use: #ifndef GOAT_PARSER / #include <immintrin.h> / #endif
		prologue.WriteString("#define GOAT_PARSER 1\n")
		// Define x86 SIMD types as opaque structs for the parser
		// The actual types are compiler built-ins, but we just need names for parsing
		prologue.WriteString("typedef struct { char _[16]; } __m128;\n")
		prologue.WriteString("typedef struct { char _[16]; } __m128d;\n")
		prologue.WriteString("typedef struct { char _[16]; } __m128i;\n")
		prologue.WriteString("typedef struct { char _[32]; } __m256;\n")
		prologue.WriteString("typedef struct { char _[32]; } __m256d;\n")
		prologue.WriteString("typedef struct { char _[32]; } __m256i;\n")
		prologue.WriteString("typedef struct { char _[64]; } __m512;\n")
		prologue.WriteString("typedef struct { char _[64]; } __m512d;\n")
		prologue.WriteString("typedef struct { char _[64]; } __m512i;\n")
	}
	ast, err := cc.Parse(cfg, []cc.Source{
		{Name: "<predefined>", Value: cfg.Predefined},
		{Name: "<builtin>", Value: cc.Builtin},
		{Name: "<prologue>", Value: prologue.String()},
		{Name: t.Source, Value: f},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to parse source file %v: %w", t.Source, err)
	}
	var functions []Function
	for tu := ast.TranslationUnit; tu != nil; tu = tu.TranslationUnit {
		externalDeclaration := tu.ExternalDeclaration
		if externalDeclaration.Position().Filename == t.Source && externalDeclaration.Case == cc.ExternalDeclarationFuncDef {
			functionSpecifier := externalDeclaration.FunctionDefinition.DeclarationSpecifiers.FunctionSpecifier
			if functionSpecifier != nil && functionSpecifier.Case == cc.FunctionSpecifierInline {
				// ignore inline functions
				continue
			}
			if function, err := t.convertFunction(externalDeclaration.FunctionDefinition); err != nil {
				return nil, err
			} else {
				functions = append(functions, function)
			}
		}
	}
	sort.Slice(functions, func(i, j int) bool {
		return functions[i].Position < functions[j].Position
	})
	return functions, nil
}

func (t *TranslateUnit) generateGoStubs(functions []Function) error {
	// generate code
	var builder strings.Builder
	builder.WriteString(buildTags)
	t.writeHeader(&builder)
	builder.WriteString(fmt.Sprintf("package %v\n", t.Package))
	if hasPointer(functions) {
		builder.WriteString("\nimport \"unsafe\"\n")
	}
	for _, function := range functions {
		builder.WriteString("\n//go:noescape\n")
		builder.WriteString("func ")
		builder.WriteString(function.Name)
		builder.WriteRune('(')
		for i, param := range function.Parameters {
			if i > 0 {
				builder.WriteString(", ")
			}
			builder.WriteString(param.Name)
			if i+1 == len(function.Parameters) || function.Parameters[i+1].String() != param.String() {
				builder.WriteRune(' ')
				builder.WriteString(param.String())
			}
		}
		builder.WriteRune(')')
		if function.Type != "void" {
			switch function.Type {
			case "_Bool":
				builder.WriteString(" (result bool)")
			case "double":
				builder.WriteString(" (result float64)")
			case "float":
				builder.WriteString(" (result float32)")
			case "int64_t", "long":
				builder.WriteString(" (result int64)")
			default:
				// Check for NEON vector types
				if sz := NeonTypeSize(function.Type); sz > 0 {
					builder.WriteString(fmt.Sprintf(" (result [%d]byte)", sz))
				} else if sz := X86SIMDTypeSize(function.Type); sz > 0 {
					// Check for x86 SIMD vector types
					builder.WriteString(fmt.Sprintf(" (result [%d]byte)", sz))
				} else {
					return fmt.Errorf("unsupported return type: %v", function.Type)
				}
			}
		}
		builder.WriteRune('\n')
	}

	// write file
	f, err := os.Create(t.Go)
	if err != nil {
		return err
	}
	defer func(f *os.File) {
		if err = f.Close(); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}(f)
	_, err = f.WriteString(builder.String())
	return err
}

func (t *TranslateUnit) compile(args ...string) error {
	args = append(args, "-mno-red-zone", "-mllvm", "-inline-threshold=1000",
		"-fno-exceptions", "-fno-rtti", "-fno-builtin",
		"-fomit-frame-pointer")
	if runtime.GOARCH == "arm64" {
		// R18 is the "platform register", reserved on the Apple platform.
		// See https://go.dev/doc/asm#arm64
		args = append(args, "-ffixed-x18")
	} else if runtime.GOARCH == "riscv64" {
		// X27 points to the Go routine structure.
		args = append(args, "-ffixed-x27")
	}
	_, err := runCommand("clang", append([]string{"-S", "-target", buildTarget, "-c", t.Source, "-o", t.Assembly}, args...)...)
	if err != nil {
		return err
	}
	_, err = runCommand("clang", append([]string{"-target", buildTarget, "-c", t.Assembly, "-o", t.Object}, args...)...)
	return err
}

func (t *TranslateUnit) Translate() error {
	functions, err := t.parseSource()
	if err != nil {
		return err
	}
	if err = t.generateGoStubs(functions); err != nil {
		return err
	}
	if err = t.compile(t.Options...); err != nil {
		return err
	}
	assembly, stackSizes, err := parseAssembly(t.Assembly)
	if err != nil {
		return err
	}
	dump, err := runCommand("objdump", "-d", t.Object, "--insn-width", "16")
	if err != nil {
		return err
	}
	err = parseObjectDump(dump, assembly)
	if err != nil {
		return err
	}
	for i, name := range functions {
		functions[i].Lines = assembly[name.Name]
		functions[i].StackSize = stackSizes[name.Name]
	}
	return t.generateGoAssembly(t.GoAssembly, functions)
}

type ParameterType struct {
	Type    string
	Pointer bool
}

func (p ParameterType) String() string {
	if p.Pointer {
		return "unsafe.Pointer"
	}
	// Check for NEON vector types first
	if sz := NeonTypeSize(p.Type); sz > 0 {
		// NEON vectors are passed as fixed-size byte arrays in Go
		return fmt.Sprintf("[%d]byte", sz)
	}
	// Check for x86 SIMD vector types
	if sz := X86SIMDTypeSize(p.Type); sz > 0 {
		// x86 SIMD vectors are passed as fixed-size byte arrays in Go
		return fmt.Sprintf("[%d]byte", sz)
	}
	switch p.Type {
	case "_Bool":
		return "bool"
	case "int64_t", "long":
		return "int64"
	case "double":
		return "float64"
	case "float":
		return "float32"
	default:
		_, _ = fmt.Fprintln(os.Stderr, "unsupported param type:", p.Type)
		os.Exit(1)
		return ""
	}
}

type Parameter struct {
	Name string
	ParameterType
}

type Function struct {
	Name       string
	Position   int
	Type       string
	Parameters []Parameter
	Lines      []Line
	StackSize  int
}

// convertFunction extracts the function definition from cc.DirectDeclarator.
func (t *TranslateUnit) convertFunction(functionDefinition *cc.FunctionDefinition) (Function, error) {
	// parse return type
	declarationSpecifiers := functionDefinition.DeclarationSpecifiers
	if declarationSpecifiers.Case != cc.DeclarationSpecifiersTypeSpec {
		return Function{}, fmt.Errorf("invalid function return type: %v", declarationSpecifiers.Case)
	}
	returnType := declarationSpecifiers.TypeSpecifier.Token.SrcStr()
	// parse parameters
	directDeclarator := functionDefinition.Declarator.DirectDeclarator
	if directDeclarator.Case != cc.DirectDeclaratorFuncParam {
		return Function{}, fmt.Errorf("invalid function parameter: %v", directDeclarator.Case)
	}
	params, err := t.convertFunctionParameters(directDeclarator.ParameterTypeList.ParameterList)
	if err != nil {
		return Function{}, err
	}
	return Function{
		Name:       directDeclarator.DirectDeclarator.Token.SrcStr(),
		Position:   directDeclarator.Position().Line,
		Type:       returnType,
		Parameters: params,
	}, nil
}

// convertFunctionParameters extracts function parameters from cc.ParameterList.
func (t *TranslateUnit) convertFunctionParameters(params *cc.ParameterList) ([]Parameter, error) {
	declaration := params.ParameterDeclaration
	paramName := declaration.Declarator.DirectDeclarator.Token.SrcStr()
	var paramType string
	if declaration.DeclarationSpecifiers.Case == cc.DeclarationSpecifiersTypeQual {
		paramType = declaration.DeclarationSpecifiers.DeclarationSpecifiers.TypeSpecifier.Token.SrcStr()
	} else {
		paramType = declaration.DeclarationSpecifiers.TypeSpecifier.Token.SrcStr()
	}
	isPointer := declaration.Declarator.Pointer != nil
	// Accept scalar types, NEON vector types, x86 SIMD types, or pointers
	if _, ok := supportedTypes[paramType]; !ok && !IsNeonType(paramType) && !IsX86SIMDType(paramType) && !isPointer {
		position := declaration.Position()
		return nil, fmt.Errorf("%v:%v:%v: error: unsupported type: %v",
			position.Filename, position.Line+t.Offset, position.Column, paramType)
	}
	paramNames := []Parameter{{
		Name: paramName,
		ParameterType: ParameterType{
			Type:    paramType,
			Pointer: isPointer,
		},
	}}
	if params.ParameterList != nil {
		if nextParamNames, err := t.convertFunctionParameters(params.ParameterList); err != nil {
			return nil, err
		} else {
			paramNames = append(paramNames, nextParamNames...)
		}
	}
	return paramNames, nil
}

func (t *TranslateUnit) writeHeader(builder *strings.Builder) {
	builder.WriteString("// Code generated by GoAT. DO NOT EDIT.\n")
	builder.WriteString("// versions:\n")
	builder.WriteString(fmt.Sprintf("// 	clang   %s\n", fetchVersion("clang")))
	builder.WriteString(fmt.Sprintf("// 	objdump %s\n", fetchVersion("objdump")))
	builder.WriteString("// flags:")
	for _, option := range t.Options {
		builder.WriteString(" ")
		builder.WriteString(option)
	}
	builder.WriteRune('\n')
	builder.WriteString(fmt.Sprintf("// source: %v\n", t.Source))
	builder.WriteRune('\n')
}

// runCommand runs a command and extract its output.
func runCommand(name string, arg ...string) (string, error) {
	if verbose {
		fmt.Fprintf(os.Stderr, "Running %v\n", append([]string{name}, arg...))
	}
	cmd := exec.Command(name, arg...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if output != nil {
			return "", errors.New(string(output))
		} else {
			return "", err
		}
	}
	return string(output), nil
}

func fetchVersion(command string) string {
	version, err := runCommand(command, "--version")
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	version = strings.Split(version, "\n")[0]
	loc := regexp.MustCompile(`\d`).FindStringIndex(version)
	if loc == nil {
		_, _ = fmt.Fprintln(os.Stderr, "failed to fetch version")
		os.Exit(1)
	}
	return version[loc[0]:]
}

func hasPointer(functions []Function) bool {
	for _, function := range functions {
		for _, param := range function.Parameters {
			if param.Pointer {
				return true
			}
		}
	}
	return false
}

var verbose bool

var command = &cobra.Command{
	Use:  "goat source [-o output_directory]",
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		output, _ := cmd.PersistentFlags().GetString("output")
		if output == "" {
			var err error
			if output, err = os.Getwd(); err != nil {
				_, _ = fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		}
		var options []string
		machineOptions, _ := cmd.PersistentFlags().GetStringSlice("machine-option")
		for _, m := range machineOptions {
			options = append(options, "-m"+m)
		}
		extraOptions, _ := cmd.PersistentFlags().GetStringSlice("extra-option")
		options = append(options, extraOptions...)
		optimizeLevel, _ := cmd.PersistentFlags().GetInt("optimize-level")
		options = append(options, fmt.Sprintf("-O%d", optimizeLevel))
		file := NewTranslateUnit(args[0], output, options...)
		if err := file.Translate(); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	},
}

func init() {
	command.PersistentFlags().StringP("output", "o", "", "output directory of generated files")
	command.PersistentFlags().StringSliceP("machine-option", "m", nil, "machine option for clang")
	command.PersistentFlags().StringSliceP("extra-option", "e", nil, "extra option for clang")
	command.PersistentFlags().IntP("optimize-level", "O", 0, "optimization level for clang")
	command.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "if set, increase verbosity level")
}

func main() {
	if err := command.Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
