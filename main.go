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
				return fmt.Errorf("unsupported return type: %v", function.Type)
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
	args = append(args, "-mno-red-zone", "-mstackrealign", "-mllvm", "-inline-threshold=1000",
		"-fno-asynchronous-unwind-tables", "-fno-exceptions", "-fno-rtti", "-fno-builtin")
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
	if _, ok := supportedTypes[paramType]; !ok && !isPointer {
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
