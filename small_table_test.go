package quamina

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestFAMergePerf(t *testing.T) {
	words := readWWords(t)
	patterns := make([]string, 0, len(words))
	for _, word := range words {
		pattern := fmt.Sprintf(`{"x": [ "%s" ] }`, string(word))
		patterns = append(patterns, pattern)
	}
	before := time.Now()
	q, _ := New()
	for _, pattern := range patterns {
		err := q.AddPattern(pattern, pattern)
		if err != nil {
			t.Error("ap: " + err.Error())
		}
	}
	elapsed := float64(time.Since(before).Milliseconds())

	for _, word := range words {
		event := fmt.Sprintf(`{"x": "%s"}`, string(word))
		matches, err := q.MatchesForEvent([]byte(event))
		if err != nil {
			t.Error("M4: " + err.Error())
		}
		if len(matches) != 1 {
			t.Errorf("wanted 1 got %d", len(matches))
		}
	}
	perSecond := float64(len(patterns)) / (elapsed / 1000.0)
	fmt.Printf("%.2f addPatterns/second with letter patterns\n\n", perSecond)
}

func TestUnpack(t *testing.T) {
	st1 := newSmallTable()
	nextState := faState{
		table:            st1,
		fieldTransitions: nil,
	}
	nextStep := faNext{states: []*faState{&nextState}}

	st := smallTable{
		ceilings: []uint8{2, 3, byte(byteCeiling)},
		steps:    []*faNext{nil, &nextStep, nil},
	}
	u := unpackTable(&st)
	for i := range u {
		if i == 2 {
			if u[i] != &nextStep {
				t.Error("Not in pos 2")
			}
		} else {
			if u[i] != nil {
				t.Errorf("Non-nil at %d", i)
			}
		}
	}
}

func unravelSmallTable(buf *bytes.Buffer, table *smallTable, depth int, showMemoryAddress bool, visited map[interface{}]bool) {
	if visited[table] {
		buf.WriteString(fmt.Sprintf("%s(already visited)\n", strings.Repeat("  ", depth)))
		return
	}
	visited[table] = true

	if table == nil {
		buf.WriteString(fmt.Sprintf("%s<nil>\n", strings.Repeat("  ", depth)))
		return
	}

	indent := strings.Repeat("  ", depth)
	if showMemoryAddress {
		buf.WriteString(fmt.Sprintf("%ssmallTable: %p\n", indent, table))
	} else {
		buf.WriteString(fmt.Sprintf("%ssmallTable:\n", indent))
	}
	buf.WriteString(fmt.Sprintf("%s  ceilings: %s\n", indent, formatCeilings(table.ceilings)))
	buf.WriteString(fmt.Sprintf("%s  steps:\n", indent))
	for i, step := range table.steps {
		if step != nil {
			if showMemoryAddress {
				buf.WriteString(fmt.Sprintf("%s    %d: %p\n", indent, i, step))
			} else {
				buf.WriteString(fmt.Sprintf("%s    %d:\n", indent, i))
			}
			unravelFaNext(buf, step, depth+3, showMemoryAddress, visited)
		} else {
			buf.WriteString(fmt.Sprintf("%s    %d: <nil>\n", indent, i))
		}
	}
	buf.WriteString(fmt.Sprintf("%s  epsilon:\n", indent))
	for i, state := range table.epsilon {
		if showMemoryAddress {
			buf.WriteString(fmt.Sprintf("%s    %d: %p\n", indent, i, state))
		} else {
			buf.WriteString(fmt.Sprintf("%s    %d:\n", indent, i))
		}
		unravelFaState(buf, state, depth+3, showMemoryAddress, visited)
	}
}

func formatCeilings(ceilings []byte) string {
	var formatted []string
	for i, ceiling := range ceilings {
		if i == len(ceilings)-1 && ceiling == byte(byteCeiling) {
			formatted = append(formatted, fmt.Sprintf("byteCeiling(0x%02X)", ceiling))
		} else if ceiling >= 32 && ceiling <= 126 {
			formatted = append(formatted, fmt.Sprintf("'%c'", ceiling))
		} else {
			formatted = append(formatted, fmt.Sprintf("0x%02X", ceiling))
		}
	}
	return "[" + strings.Join(formatted, ", ") + "]"
}
