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
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
)

// DataSymbol is a constant data object collected from compiler-generated data
// sections and emitted as Go asm DATA/GLOBL directives.
type DataSymbol struct {
	Name string
	Data []byte
}

// ParseDataDirective parses data directives that embed literal bytes in clang
// assembly output.
func ParseDataDirective(line string) ([]byte, bool, error) {
	line = strings.TrimSpace(line)
	if strings.HasPrefix(line, ".ascii") || strings.HasPrefix(line, ".asciz") {
		parts := strings.Fields(line)
		if len(parts) < 2 {
			return nil, false, fmt.Errorf("invalid ascii directive: %s", line)
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, parts[0]))
		decoded, err := strconv.Unquote(value)
		if err != nil {
			return nil, false, err
		}
		data := []byte(decoded)
		if strings.HasPrefix(line, ".asciz") {
			data = append(data, 0)
		}
		return data, true, nil
	}
	return nil, false, nil
}

// GenerateDataSymbols emits Go asm DATA/GLOBL directives for data symbols.
func GenerateDataSymbols(symbols []DataSymbol, byteOrder binary.ByteOrder) string {
	var builder strings.Builder
	for _, symbol := range symbols {
		for offset := 0; offset < len(symbol.Data); {
			remaining := len(symbol.Data) - offset
			size := 8
			if remaining < size {
				size = remaining
			}
			var value uint64
			for i := 0; i < size; i++ {
				if byteOrder == binary.BigEndian {
					value = (value << 8) | uint64(symbol.Data[offset+i])
				} else {
					value |= uint64(symbol.Data[offset+i]) << (8 * i)
				}
			}
			builder.WriteString(fmt.Sprintf("DATA %s<>+0x%03x(SB)/%d, $0x%0*x\n", symbol.Name, offset, size, size*2, value))
			offset += size
		}
		builder.WriteString(fmt.Sprintf("GLOBL %s<>(SB), 8, $%d\n\n", symbol.Name, len(symbol.Data)))
	}
	return builder.String()
}
