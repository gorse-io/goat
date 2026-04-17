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
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/gorse-io/goat/internal"
	_ "github.com/gorse-io/goat/internal/amd64"
	_ "github.com/gorse-io/goat/internal/arm64"
	_ "github.com/gorse-io/goat/internal/loong64"
	_ "github.com/gorse-io/goat/internal/riscv64"
	"github.com/spf13/cobra"
)

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

		targetName, _ := cmd.PersistentFlags().GetString("target")
		target, ok := internal.LookupTarget(targetName)
		if !ok {
			_, _ = fmt.Fprintf(os.Stderr, "unsupported target: %s (supported: %s)\n",
				targetName, strings.Join(internal.TargetNames(), ", "))
			os.Exit(1)
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

		internal.SetVerbose(verbose)
		file := internal.NewTranslateUnit(args[0], output, target, options...)
		if err := file.Translate(); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	},
}

func init() {
	command.PersistentFlags().StringP("output", "o", "", "output directory of generated files")
	command.PersistentFlags().StringP("target", "t", runtime.GOARCH, "target architecture, using Go GOARCH names")
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
