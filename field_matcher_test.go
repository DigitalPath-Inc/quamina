package quamina

import (
	"bytes"
	"fmt"
	"strings"
)

func unravelFieldMatcher(buf *bytes.Buffer, fm *fieldMatcher, depth int, showMemoryAddress bool) {
	indent := strings.Repeat("  ", depth)
	fields := fm.fields()

	if showMemoryAddress {
		buf.WriteString(fmt.Sprintf("%sstate: %p\n", indent, fm))
	} else {
		buf.WriteString(fmt.Sprintf("%sstate:\n", indent))
	}

	if len(fields.transitions) > 0 {
		buf.WriteString(fmt.Sprintf("%s  transitions:\n", indent))
		for path, vm := range fields.transitions {
			if showMemoryAddress {
				buf.WriteString(fmt.Sprintf("%s    %s: %p\n", indent, path, vm))
			} else {
				buf.WriteString(fmt.Sprintf("%s    %s:\n", indent, path))
			}
			unravelValueMatcher(buf, vm, depth+3, showMemoryAddress)
		}
	}

	if len(fields.matches) > 0 {
		buf.WriteString(fmt.Sprintf("%s  matches: %v\n", indent, fields.matches))
	}

	if len(fields.existsTrue) > 0 {
		buf.WriteString(fmt.Sprintf("%s  existsTrue:\n", indent))
		for path, fm := range fields.existsTrue {
			if showMemoryAddress {
				buf.WriteString(fmt.Sprintf("%s    %s: %p\n", indent, path, fm))
			} else {
				buf.WriteString(fmt.Sprintf("%s    %s:\n", indent, path))
			}
			unravelFieldMatcher(buf, fm, depth+3, showMemoryAddress)
		}
	}

	if len(fields.existsFalse) > 0 {
		buf.WriteString(fmt.Sprintf("%s  existsFalse:\n", indent))
		for path, fm := range fields.existsFalse {
			if showMemoryAddress {
				buf.WriteString(fmt.Sprintf("%s    %s: %p\n", indent, path, fm))
			} else {
				buf.WriteString(fmt.Sprintf("%s    %s:\n", indent, path))
			}
			unravelFieldMatcher(buf, fm, depth+3, showMemoryAddress)
		}
	}
}
