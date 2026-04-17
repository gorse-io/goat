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
package parser

import (
	"sort"
	"sync"
)

type Target struct {
	GOARCH             string
	BuildTags          string
	ClangTriple        string
	Prologue           string
	ClangOptions       []string
	ParseAssembly      func(string) (any, map[string]int, error)
	ParseObjectDump    func(string, any) error
	GenerateGoAssembly func(string, string, string, []Function, any) error
}

var (
	targetMu sync.RWMutex
	targets  = make(map[string]Target)
)

func RegisterTarget(name string, target Target) {
	targetMu.Lock()
	defer targetMu.Unlock()
	targets[name] = target
}

func LookupTarget(name string) (Target, bool) {
	targetMu.RLock()
	defer targetMu.RUnlock()
	target, ok := targets[name]
	return target, ok
}

func TargetNames() []string {
	targetMu.RLock()
	defer targetMu.RUnlock()
	names := make([]string, 0, len(targets))
	for name := range targets {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
