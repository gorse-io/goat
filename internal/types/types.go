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

package types

import "fmt"

// Line represents a line of assembly code
type Line struct {
	Labels   []string
	Assembly string
	Binary   []string
}

// SupportedTypes maps C types to their sizes in bytes
var SupportedTypes = map[string]int{
	"int64_t": 8,
	"long":    8,
	"float":   4,
	"double":  8,
	"_Bool":   1,
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
		panic(fmt.Sprintf("unsupported param type: %s", p.Type))
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