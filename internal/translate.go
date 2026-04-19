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
package internal

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

var SupportedTypes = map[string]int{
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
	Target     Target
}

func NewTranslateUnit(source string, outputDir string, target Target, options ...string) TranslateUnit {
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
		Target:     target,
	}
}

// ParseSource parses the C source file and extracts function declarations.
func (t *TranslateUnit) ParseSource() ([]Function, error) {
	clangPath := GetClangPath()
	args := []string{"-target", t.Target.ClangTriple}
	args = append(args, t.Target.ClangOptions...)
	args = append(args, t.Options...)
	args = append(args, "-Xclang", "-ast-dump=json", "-fsyntax-only", t.Source)

	output, err := RunCommand(clangPath, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to parse source file %v: %w", t.Source, err)
	}

	var root clangASTNode
	if err := json.Unmarshal([]byte(output), &root); err != nil {
		return nil, fmt.Errorf("failed to decode clang AST for %v: %w", t.Source, err)
	}

	functions := make([]Function, 0)
	if err := t.collectClangFunctions(&root, &functions); err != nil {
		return nil, err
	}
	sort.Slice(functions, func(i, j int) bool {
		return functions[i].Position < functions[j].Position
	})
	return functions, nil
}

func (t *TranslateUnit) GenerateGoStubs(functions []Function) error {
	var builder strings.Builder
	builder.WriteString(t.Target.BuildTags)
	builder.WriteString(t.Header())
	builder.WriteString(fmt.Sprintf("package %v\n", t.Package))
	if HasPointer(functions) {
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
	args = append(args, "-mllvm", "-inline-threshold=1000",
		"-fno-asynchronous-unwind-tables", "-fno-exceptions", "-fno-rtti", "-fno-builtin")
	args = append(args, t.Target.ClangOptions...)
	compileArgs := []string{"-target", t.Target.ClangTriple}
	clangPath := GetClangPath()
	_, err := RunCommand(clangPath, append(append([]string{"-S"}, compileArgs...), append([]string{"-c", t.Source, "-o", t.Assembly}, args...)...)...)
	if err != nil {
		return err
	}
	_, err = RunCommand(clangPath, append(compileArgs, append([]string{"-c", t.Assembly, "-o", t.Object}, args...)...)...)
	return err
}

func (t *TranslateUnit) Translate() error {
	functions, err := t.ParseSource()
	if err != nil {
		return err
	}
	if err = t.GenerateGoStubs(functions); err != nil {
		return err
	}
	if err = t.compile(t.Options...); err != nil {
		return err
	}
	assembly, stackSizes, err := t.Target.ParseAssembly(t.Assembly)
	if err != nil {
		return err
	}
	dump, err := RunCommand(GetObjdumpPath(t.Target), "-d", t.Object, "--insn-width", "16")
	if err != nil {
		return err
	}
	if err = t.Target.ParseObjectDump(dump, assembly); err != nil {
		return err
	}
	for i, function := range functions {
		functions[i].Lines = assembly[function.Name]
		functions[i].StackSize = stackSizes[function.Name]
	}
	return t.Target.GenerateGoAssembly(t.Target.BuildTags, t.Header(), t.GoAssembly, functions)
}

func (t *TranslateUnit) Header() string {
	var builder strings.Builder
	builder.WriteString("// Code generated by GoAT. DO NOT EDIT.\n")
	builder.WriteString("// versions:\n")
	builder.WriteString(fmt.Sprintf("// \tclang   %s\n", FetchVersion(GetClangPath())))
	builder.WriteString(fmt.Sprintf("// \tobjdump %s\n", FetchVersion(GetObjdumpPath(t.Target))))
	builder.WriteString("// flags:")
	for _, option := range t.Options {
		builder.WriteString(" ")
		builder.WriteString(option)
	}
	builder.WriteRune('\n')
	builder.WriteString(fmt.Sprintf("// source: %v\n", t.Source))
	builder.WriteRune('\n')
	return builder.String()
}

// RunCommand runs a command and extract its output.
func RunCommand(name string, arg ...string) (string, error) {
	if verbose {
		fmt.Fprintf(os.Stderr, "Running %v\n", append([]string{name}, arg...))
	}
	cmd := exec.Command(name, arg...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", errors.New(stderr.String())
	}
	return stdout.String(), nil
}

func FetchVersion(command string) string {
	version, err := RunCommand(command, "--version")
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

func HasPointer(functions []Function) bool {
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

func SetVerbose(v bool) {
	verbose = v
}

// GetClangPath returns the path to the clang executable.
// If the CLANG environment variable is set, it uses that path;
// otherwise, it defaults to "clang".
func GetClangPath() string {
	path := os.Getenv("CLANG")
	if path != "" {
		return path
	}
	return "clang"
}

// GetObjdumpPath returns the path to the target-specific objdump executable.
// If OBJDUMP is set, it must resolve to an executable. Otherwise, GoAT requires
// the canonical cross-toolchain objdump for the target architecture.
func GetObjdumpPath(target Target) string {
	path := os.Getenv("OBJDUMP")
	if path != "" {
		return path
	}
	if target.GOARCH == runtime.GOARCH {
		return "objdump"
	}
	return target.ClangTriple + "-objdump"
}
