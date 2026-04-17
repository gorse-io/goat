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

	"modernc.org/cc/v4"
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

// convertFunction extracts the function definition from cc.DirectDeclarator.
func (t *TranslateUnit) convertFunction(functionDefinition *cc.FunctionDefinition) (Function, error) {
	declarationSpecifiers := functionDefinition.DeclarationSpecifiers
	if declarationSpecifiers.Case != cc.DeclarationSpecifiersTypeSpec {
		return Function{}, fmt.Errorf("invalid function return type: %v", declarationSpecifiers.Case)
	}
	returnType := declarationSpecifiers.TypeSpecifier.Token.SrcStr()
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
	if _, ok := SupportedTypes[paramType]; !ok && !isPointer {
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
		nextParamNames, err := t.convertFunctionParameters(params.ParameterList)
		if err != nil {
			return nil, err
		}
		paramNames = append(paramNames, nextParamNames...)
	}
	return paramNames, nil
}
