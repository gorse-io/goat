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
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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

type Line struct {
	Labels   []string
	Assembly string
	Binary   string
}

type Function struct {
	Name       string
	Position   int
	Type       string
	Parameters []Parameter
	Lines      []Line
	StackSize  int
}

type clangASTNode struct {
	Kind   string         `json:"kind"`
	Name   string         `json:"name"`
	Type   *clangASTType  `json:"type"`
	Loc    clangASTLoc    `json:"loc"`
	Inline bool           `json:"inline"`
	Inner  []clangASTNode `json:"inner"`
}

type clangASTType struct {
	QualType string `json:"qualType"`
}

type clangASTLoc struct {
	File         string       `json:"file"`
	Line         int          `json:"line"`
	IncludedFrom *clangASTLoc `json:"includedFrom"`
}

func (t *TranslateUnit) collectClangFunctions(node *clangASTNode, functions *[]Function) error {
	if node.Kind == "FunctionDecl" {
		function, ok, err := t.convertClangFunction(node)
		if err != nil {
			return err
		}
		if ok {
			*functions = append(*functions, function)
		}
	}
	for i := range node.Inner {
		if err := t.collectClangFunctions(&node.Inner[i], functions); err != nil {
			return err
		}
	}
	return nil
}

func (t *TranslateUnit) convertClangFunction(node *clangASTNode) (Function, bool, error) {
	if !t.isSourceFunction(node) || node.Inline {
		return Function{}, false, nil
	}

	params := make([]Parameter, 0)
	for i := range node.Inner {
		child := node.Inner[i]
		if child.Kind != "ParmVarDecl" {
			continue
		}
		if child.Type == nil {
			return Function{}, false, fmt.Errorf("missing parameter type for function %v", node.Name)
		}
		paramType, isPointer := parseClangQualType(child.Type.QualType)
		if _, ok := SupportedTypes[paramType]; !ok && !isPointer {
			line := child.Loc.Line
			if line == 0 {
				line = node.Loc.Line
			}
			return Function{}, false, fmt.Errorf("%v:%v:1: error: unsupported type: %v", t.Source, line+t.Offset, paramType)
		}
		name := child.Name
		if name == "" {
			name = fmt.Sprintf("arg%d", len(params)+1)
		}
		params = append(params, Parameter{
			Name: name,
			ParameterType: ParameterType{
				Type:    paramType,
				Pointer: isPointer,
			},
		})
	}

	returnType, _ := parseClangQualType(clangFunctionReturnType(node))
	return Function{
		Name:       node.Name,
		Position:   node.Loc.Line,
		Type:       returnType,
		Parameters: params,
	}, true, nil
}

func (t *TranslateUnit) isSourceFunction(node *clangASTNode) bool {
	if node.Name == "" || node.Loc.Line == 0 || node.Loc.IncludedFrom != nil {
		return false
	}
	hasBody := false
	for i := range node.Inner {
		if node.Inner[i].Kind == "CompoundStmt" {
			hasBody = true
			break
		}
	}
	if !hasBody {
		return false
	}
	return node.Loc.File == "" || filepath.Clean(node.Loc.File) == filepath.Clean(t.Source)
}

func clangFunctionReturnType(node *clangASTNode) string {
	if node.Type == nil {
		return ""
	}
	qualType := node.Type.QualType
	idx := strings.IndexRune(qualType, '(')
	if idx == -1 {
		return strings.TrimSpace(qualType)
	}
	return strings.TrimSpace(qualType[:idx])
}

func parseClangQualType(qualType string) (string, bool) {
	qualType = strings.TrimSpace(qualType)
	isPointer := strings.Contains(qualType, "*")
	qualType = strings.ReplaceAll(qualType, "*", " ")
	parts := strings.Fields(qualType)
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		switch part {
		case "const", "volatile", "restrict", "static", "register":
			continue
		default:
			filtered = append(filtered, part)
		}
	}
	return strings.Join(filtered, " "), isPointer
}
