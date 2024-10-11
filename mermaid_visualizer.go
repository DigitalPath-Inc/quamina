package quamina

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
)

type mermaidVisualizer struct {
	buf          *bytes.Buffer
	visitedNodes map[uintptr]string
	visitedLinks map[string]bool
}

func newMermaidVisualizer() *mermaidVisualizer {
	return &mermaidVisualizer{
		buf:          &bytes.Buffer{},
		visitedNodes: make(map[uintptr]string),
		visitedLinks: make(map[string]bool),
	}
}

func (mv *mermaidVisualizer) visualize(node interface{}) string {
	mv.buf.WriteString("graph TD\n")
	mv.visualizeNode(node, "")
	return mv.buf.String()
}

func (mv *mermaidVisualizer) visualizeNode(node interface{}, parentId string) string {
	if node == nil {
		return ""
	}

	ptr := reflect.ValueOf(node).Pointer()
	currentId, exists := mv.visitedNodes[ptr]

	if !exists {
		currentId = fmt.Sprintf("%p", node)
		mv.visitedNodes[ptr] = currentId

		label := mv.getLabel(node)
		mv.buf.WriteString(fmt.Sprintf("    %s[\"%s\"]\n", currentId, sanitizeMermaidLabel(label)))

		mv.visualizeChildren(node, currentId)
	}

	if parentId != "" {
		linkKey := fmt.Sprintf("%s-->%s", parentId, currentId)
		if !mv.visitedLinks[linkKey] {
			mv.buf.WriteString(fmt.Sprintf("    %s --> %s\n", parentId, currentId))
			mv.visitedLinks[linkKey] = true
		}
	}

	return currentId
}

func (mv *mermaidVisualizer) getLabel(node interface{}) string {
	t := reflect.TypeOf(node)
	label := t.String()

	switch v := node.(type) {
	case *coreMatcher:
		label = fmt.Sprintf("CoreMatcher<br />%p", v)
	case *coreFields:
		label = fmt.Sprintf("CoreFields<br />%p", v)
	case *fieldMatcher:
		label = fmt.Sprintf("FieldMatcher<br />%p", v)
		if len(v.fields().matches) > 0 {
			patterns := make([]string, 0, len(v.fields().matches))
			for pattern := range v.fields().matches {
				patterns = append(patterns, fmt.Sprintf("%v", pattern))
			}
			label += fmt.Sprintf("<br />Patterns: %s", strings.Join(patterns, ", "))
		}
	case *valueMatcher:
		label = fmt.Sprintf("ValueMatcher<br />%p", v)
	case *smallTable:
		label = fmt.Sprintf("SmallTable<br />%p", v)
	case *faNext:
		// label = fmt.Sprintf("FaNext<br />%p", v)
	case *faState:
		// label = fmt.Sprintf("FaState<br />%p", v)
		if len(v.fieldTransitions) > 0 {
			patterns := make([]string, 0)
			for _, fm := range v.fieldTransitions {
				for pattern := range fm.fields().matches {
					patterns = append(patterns, fmt.Sprintf("%v", pattern))
				}
			}
			if len(patterns) > 0 {
				label += fmt.Sprintf("<br />Patterns: %s", strings.Join(patterns, ", "))
			}
		}
	}

	return label
}

func (mv *mermaidVisualizer) visualizeChildren(node interface{}, parentId string) {
	switch v := node.(type) {
	case *coreMatcher:
		mv.visualizeNode(v.fields(), parentId)
	case *coreFields:
		mv.visualizeNode(v.state, parentId)
		mv.visualizeNode(v.segmentsTree, parentId)
		mv.visualizeNode(v.nfaMeta, parentId)
	case *fieldMatcher:
		fields := v.fields()
		for path, vm := range fields.transitions {
			vmId := mv.visualizeNode(vm, parentId)
			mv.buf.WriteString(fmt.Sprintf("    %s[\"%s\"]\n", vmId, sanitizeMermaidLabel(path)))
		}
		for path, subFm := range fields.existsTrue {
			subFmId := mv.visualizeNode(subFm, parentId)
			mv.buf.WriteString(fmt.Sprintf("    %s[\"%s\"]\n", subFmId, sanitizeMermaidLabel(fmt.Sprintf("ExistsTrue_%s", path))))
		}
		for path, subFm := range fields.existsFalse {
			subFmId := mv.visualizeNode(subFm, parentId)
			mv.buf.WriteString(fmt.Sprintf("    %s[\"%s\"]\n", subFmId, sanitizeMermaidLabel(fmt.Sprintf("ExistsFalse_%s", path))))
		}
	case *valueMatcher:
		fields := v.fields()
		if fields.startTable != nil {
			mv.visualizeNode(fields.startTable, parentId)
		}
		if fields.singletonTransition != nil {
			mv.visualizeNode(fields.singletonTransition, parentId)
		}
	case *smallTable:
		for i, step := range v.steps {
			if step != nil {
				stepId := mv.visualizeNode(step, parentId)
				char := fmt.Sprintf("%c", v.ceilings[i]-1)
				if v.ceilings[i] < 32 || v.ceilings[i] > 126 {
					char = fmt.Sprintf("0x%02X", v.ceilings[i])
				}
				mv.buf.WriteString(fmt.Sprintf("    %s[\"%s\"]\n", stepId, sanitizeMermaidLabel(char)))
			}
		}
		for i, state := range v.epsilon {
			stateId := mv.visualizeNode(state, parentId)
			mv.buf.WriteString(fmt.Sprintf("    %s[\"%s\"]\n", stateId, sanitizeMermaidLabel(fmt.Sprintf("Epsilon_%d", i))))
		}
	case *faNext:
		for _, state := range v.states {
			mv.visualizeNode(state, parentId)
		}
	case *faState:
		if v.table != nil {
			mv.visualizeNode(v.table, parentId)
		}
		for _, fm := range v.fieldTransitions {
			fmId := mv.visualizeNode(fm, parentId)
			// mv.buf.WriteString(fmt.Sprintf("    %s[\"%s\"]\n", fmId, sanitizeMermaidLabel(fmt.Sprintf("FieldTransition_%d", i))))

			// Add patterns as a separate node
			if len(fm.fields().matches) > 0 {
				patternsId := fmt.Sprintf("%s_patterns", fmId)
				patterns := getPatternStrings(fm.fields().matches)
				mv.buf.WriteString(fmt.Sprintf("    %s[\"%s\"]\n", patternsId, sanitizeMermaidLabel(fmt.Sprintf("Patterns: %s", patterns))))
				mv.buf.WriteString(fmt.Sprintf("    %s --> %s\n", fmId, patternsId))
			}
		}
	}
}

// Helper function to get pattern strings
func getPatternStrings(matches []X) string {
	patterns := make([]string, 0, len(matches))
	for _, pattern := range matches {
		patterns = append(patterns, fmt.Sprintf("%v", pattern))
	}
	return strings.Join(patterns, ", ")
}

// Helper function to sanitize labels for Mermaid
func sanitizeMermaidLabel(label string) string {
	label = strings.ReplaceAll(label, "#", "special-#")
	label = strings.ReplaceAll(label, "\"", "QUOTE")
	label = strings.ReplaceAll(label, "`", "TICK")
	return label
}

func visualizeAsMermaid(root interface{}) string {
	mv := newMermaidVisualizer()
	return mv.visualize(root)
}
