package quamina

import (
	"bytes"
	"fmt"
	"strings"
)

func unravelFieldMatcher(buf *bytes.Buffer, fm *fieldMatcher, depth int) {
	indent := strings.Repeat("  ", depth)
	fields := fm.fields()

	buf.WriteString(fmt.Sprintf("%sstate:\n", indent))

	if len(fields.transitions) > 0 {
		buf.WriteString(fmt.Sprintf("%s  transitions:\n", indent))
		for path, vm := range fields.transitions {
			buf.WriteString(fmt.Sprintf("%s    %s:\n", indent, path))
			unravelValueMatcher(buf, vm, depth+3)
		}
	}

	if len(fields.matches) > 0 {
		buf.WriteString(fmt.Sprintf("%s  matches: %v\n", indent, fields.matches))
	}

	if len(fields.existsTrue) > 0 {
		buf.WriteString(fmt.Sprintf("%s  existsTrue:\n", indent))
		for path, fm := range fields.existsTrue {
			buf.WriteString(fmt.Sprintf("%s    %s:\n", indent, path))
			unravelFieldMatcher(buf, fm, depth+3)
		}
	}

	if len(fields.existsFalse) > 0 {
		buf.WriteString(fmt.Sprintf("%s  existsFalse:\n", indent))
		for path, fm := range fields.existsFalse {
			buf.WriteString(fmt.Sprintf("%s    %s:\n", indent, path))
			unravelFieldMatcher(buf, fm, depth+3)
		}
	}
}
