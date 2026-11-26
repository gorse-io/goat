package arm64

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gorse-io/goat/internal"
)

func TestUsesStackPointer(t *testing.T) {
	tests := []struct {
		name     string
		assembly string
		want     bool
	}{
		{name: "sub sp", assembly: "sub\tsp, sp, #16", want: true},
		{name: "add sp", assembly: "add\tsp, sp, #16", want: true},
		{name: "str sp offset", assembly: "str\ts0, [sp, #8]", want: true},
		{name: "stp pre-index", assembly: "stp\tx29, x30, [sp, #-16]!", want: true},
		{name: "wsp register", assembly: "mov\tw0, wsp", want: true},
		{name: "register arithmetic", assembly: "add\tx0, x0, x1", want: false},
		{name: "mnemonic containing sp", assembly: "casp\tx0, x1, x2, x3, [x4]", want: false},
		{name: "sysreg containing sp", assembly: "mrs\tx0, sp_el0", want: false},
		{name: "adrp symbol named sp", assembly: "adrp\tx0, sp", want: false},
		{name: "lo12 symbol named sp", assembly: "add\tx0, x0, :lo12:sp", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := usesStackPointer([]internal.Line{{Assembly: tc.assembly}})
			if got != tc.want {
				t.Fatalf("usesStackPointer(%q) = %v, want %v", tc.assembly, got, tc.want)
			}
		})
	}
}

func TestGenerateGoAssemblyFrameReservation(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "out.s")
	functions := []internal.Function{
		{
			Name: "leaf_no_sp",
			Type: "long",
			Parameters: []internal.Parameter{
				{Name: "a", Type: "long"},
				{Name: "b", Type: "long"},
			},
			Lines: []internal.Line{
				{Assembly: "add\tx0, x0, x1", Binary: "8b010000"},
				{Assembly: "ret", Binary: "d65f03c0"},
			},
		},
		{
			Name: "leaf_with_sp",
			Type: "long",
			Parameters: []internal.Parameter{
				{Name: "a", Type: "long"},
				{Name: "b", Type: "long"},
			},
			Lines: []internal.Line{
				{Assembly: "sub\tsp, sp, #16", Binary: "d10043ff"},
				{Assembly: "str\tx0, [sp, #8]", Binary: "f90007e0"},
				{Assembly: "add\tx0, x0, x1", Binary: "8b010000"},
				{Assembly: "add\tsp, sp, #16", Binary: "910043ff"},
				{Assembly: "ret", Binary: "d65f03c0"},
			},
		},
		{
			Name: "stack_args_with_sp",
			Type: "long",
			Parameters: []internal.Parameter{
				{Name: "x1", Type: "long"},
				{Name: "x2", Type: "long"},
				{Name: "x3", Type: "long"},
				{Name: "x4", Type: "long"},
				{Name: "x5", Type: "long"},
				{Name: "x6", Type: "long"},
				{Name: "x7", Type: "long"},
				{Name: "x8", Type: "long"},
				{Name: "x9", Type: "long"},
			},
			Lines: []internal.Line{
				{Assembly: "sub\tsp, sp, #16", Binary: "d10043ff"},
				{Assembly: "add\tsp, sp, #16", Binary: "910043ff"},
				{Assembly: "ret", Binary: "d65f03c0"},
			},
		},
	}

	if err := generateGoAssembly("//go:build arm64\n", "", outPath, functions); err != nil {
		t.Fatalf("generateGoAssembly failed: %v", err)
	}
	content, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read generated assembly: %v", err)
	}
	asm := string(content)
	if !strings.Contains(asm, "TEXT ·leaf_no_sp(SB), $0-24") {
		t.Fatalf("expected $0-24 for leaf_no_sp, got:\n%s", asm)
	}
	if !strings.Contains(asm, "TEXT ·leaf_with_sp(SB), $16-24") {
		t.Fatalf("expected $16-24 for leaf_with_sp, got:\n%s", asm)
	}
	if !strings.Contains(asm, "TEXT ·stack_args_with_sp(SB), $8-80") {
		t.Fatalf("expected $8-80 for stack_args_with_sp, got:\n%s", asm)
	}
}
