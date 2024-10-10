package quamina

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/assert"
)

func TestTrieFromPatterns(t *testing.T) {
	// patternsJSON := generatePatterns(2, 2, 2)
	// patternsJSON := []string{
	// 	`{"field_0":["kiXm", "blah"]}`,
	// 	`{"field_0":["kiXm", "foo"]}`,
	// }
	patterns := make(map[X]string)
	// for i, pattern := range patternsJSON {
	// 	patterns[X(fmt.Sprintf("pattern_%d", i))] = pattern
	// }
	patterns[X("pattern_0")] = `{"field_0":["foo", "bar"], "field_1":["asdf", "qwer"]}`
	patterns[X("pattern_1")] = `{"field_0":["foo", "baz"], "field_1":["asdf", "zxcv"]}`
	patterns[X("pattern_2")] = `{"field_0":[{"prefix": "foo"}], "field_1":[{"prefix": "bar"}]}`
	patterns[X("pattern_3")] = `{"field_0":[{"equals-ignore-case": "fOo"}]}`
	patterns[X("pattern_4")] = `{"field_0":["aaaa", "abaa"], "field_1":["cccc", "cbaa"]}`
	patterns[X("pattern_5")] = `{"field_1":["bbbb", "bbaa"]}`
	patterns[X("pattern_6")] = `{"field_1":[{"prefix": "bbaa"}]}`
	patterns[X("pattern_7")] = `{"field_1":["bbbbb", "bbaa"]}`

	fmt.Printf("Patterns: %v\n", patterns)

	start := time.Now()
	fields := make(map[string]struct{})
	tries, err := trieFromPatterns(patterns, &fields)
	if err != nil {
		t.Fatalf("Error building trie: %v", err)
	}
	t.Logf("Time to build trie: %v", time.Since(start))

	t.Logf("Trie:\n%v", visualizePathTrie(tries["field_0"]))
	t.Logf("Trie:\n%v", visualizePathTrie(tries["field_1"]))
	t.Logf("Mermaid Diagram for all tries:\n%v", visualizeTriesAsMermaid(tries))
}

func TestMatcherFromPatterns(t *testing.T) {
	testCases := []struct {
		name     string
		patterns map[X]string
		events   [][]byte
		expected [][]X
	}{
		{
			name: "Multiple events and matches",
			patterns: map[X]string{
				X("pattern_0"): `{"field_0":["foo", "bar"], "field_1":["asdf", "qwer"]}`,
				X("pattern_1"): `{"field_0":["baz", "qux"], "field_1":["asdf", "zxcv"]}`,
			},
			events: [][]byte{
				[]byte(`{"field_0": "foo", "field_1": "asdf"}`),
				[]byte(`{"field_0": "baz", "field_1": "asdf"}`),
				[]byte(`{"field_0": "foo", "field_1": "zxcv"}`),
			},
			expected: [][]X{
				{X("pattern_0")},
				{X("pattern_1")},
				{},
			},
		},
		// {
		// 	name: "Different types of patterns",
		// 	patterns: map[X]string{
		// 		X("pattern_0"): `{"field_0":[{"prefix": "bar"}], "field_1":["foo", {"prefix": "bar"}, "baz", {"equals-ignore-case": "pReSeNt"}]}`,
		// 		X("pattern_1"): `{"field_0":[{"equals-ignore-case": "fOo"}], "field_1":["foo", {"prefix": "bar"}, "baz"]}`,
		// 		X("pattern_2"): `{"field_0":["foo", "bar"]}`,
		// 		X("pattern_3"): `{"field_1":["foo", "bar"]}`,
		// 		X("pattern_4"): `{"field_0":[{"equals-ignore-case": "Hello"}], "field_1":[{"prefix": "image"}]}`,
		// 		X("pattern_5"): `{"field_0":[{"prefix": "test"}], "field_1":[{"prefix": "error"}]}`,
		// 		X("pattern_6"): `{"field_0":["xyz", "abc"], "field_1":[{"equals-ignore-case": "ErRoR"}]}`,
		// 		X("pattern_7"): `{"field_0":[{"prefix": "debug"}], "field_1":[{"prefix": "debug"}]}`,
		// 	},
		// 	events: [][]byte{
		// 		[]byte(`{"field_0": "foo", "field_1": "asdf"}`),
		// 		[]byte(`{"field_0": "barcode", "field_1": "bar"}`),
		// 		[]byte(`{"field_0": "hello", "field_1": "image.jpg"}`),
		// 		[]byte(`{"field_0": "testcase", "field_1": "error.log"}`),
		// 		[]byte(`{"field_0": "xyz", "field_1": "ERROR"}`),
		// 		[]byte(`{"field_0": "HELLO", "field_1": "image.png"}`),
		// 		[]byte(`{"field_0": "debugcase", "field_1": "debug_info"}`),
		// 	},
		// 	expected: [][]X{
		// 		{X("pattern_2")},
		// 		{X("pattern_0"), X("pattern_3")},
		// 		{X("pattern_4")},
		// 		{X("pattern_5")},
		// 		{X("pattern_6")},
		// 		{X("pattern_4")},
		// 		{X("pattern_7")},
		// 	},
		// },
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			matcher, err := MatcherFromPatterns(tc.patterns)
			assert.NoError(t, err)

			oldMatcher := newCoreMatcher()
			for x, pattern := range tc.patterns {
				oldMatcher.addPattern(x, pattern)
			}

			visualizer := newMermaidVisualizer()
			t.Logf("Matcher: %v", visualizer.visualize(matcher))

			for i, event := range tc.events {
				matches, err := matcher.matchesForJSONEvent(event)
				assert.NoError(t, err)
				oldMatches, err := oldMatcher.matchesForJSONEvent(event)
				assert.NoError(t, err)
				assert.Equal(t, tc.expected[i], matches)
				if !assert.Equal(t, oldMatches, matches) {
					t.Logf("Patterns: %v, Event: %v, Matches: %v", tc.patterns, string(event), matches)
					t.Logf("Matcher: %v", unravelMatcher(matcher, true))
					t.Logf("Old Matcher: %v", unravelMatcher(oldMatcher, true))
				}
			}
		})
	}
}

func TestMatcherFromSimplePatterns(t *testing.T) {
	patterns := make(map[X]string)
	patterns[X("pattern_0")] = `{"field_0":["foo", "bar"], "field_1":["asdf", "qwer"]}`
	patterns[X("pattern_1")] = `{"field_0":["baz", "qux"], "field_1":["asdf", "zxcv"]}`

	matcher, err := MatcherFromPatterns(patterns)
	assert.NoError(t, err)

	matches, err := matcher.matchesForJSONEvent([]byte(`{"field_0": "foo", "field_1": "asdf"}`))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(matches))
	assert.Equal(t, X("pattern_0"), matches[0])

	matches, err = matcher.matchesForJSONEvent([]byte(`{"field_0": "baz", "field_1": "asdf"}`))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(matches))
	assert.Equal(t, X("pattern_1"), matches[0])

	matches, err = matcher.matchesForJSONEvent([]byte(`{"field_0": "qux", "field_1": "zxcv"}`))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(matches))
	assert.Equal(t, X("pattern_1"), matches[0])

	matches, err = matcher.matchesForJSONEvent([]byte(`{"field_0": "foo", "field_1": "zxcv"}`))
	assert.NoError(t, err)
	assert.Equal(t, 0, len(matches))
}

// func TestMatcherFromPatterns(t *testing.T) {
// 	// Generate patterns
// 	patternsJSON := generatePatterns(100, []int{750, 3})
// 	patterns := make(map[X]string)
// 	for i, pattern := range patternsJSON {
// 		patterns[X(fmt.Sprintf("pattern_%d", i))] = pattern
// 	}

// 	// Build matcher
// 	matcher, err := MatcherFromPatterns(patterns)
// 	assert.NoError(t, err)

// 	// Generate 10 events that should match
// 	matchingEvents := make([]string, 10)
// 	for i := 0; i < 10; i++ {
// 		var event map[string]interface{}
// 		err := json.Unmarshal([]byte(patternsJSON[i]), &event)
// 		assert.NoError(t, err)
// 		// Ensure we're using one of the values from the pattern
// 		for field, values := range event {
// 			if valuesSlice, ok := values.([]interface{}); ok && len(valuesSlice) > 0 {
// 				event[field] = valuesSlice[0]
// 			}
// 		}
// 		eventJSON, err := json.Marshal(event)
// 		assert.NoError(t, err)
// 		matchingEvents[i] = string(eventJSON)
// 	}

// 	// Test matching events
// 	for _, event := range matchingEvents {
// 		matches, err := matcher.matchesForJSONEvent([]byte(event))
// 		assert.NoError(t, err)

// 		assert.NotEmpty(t, matches, "Expected event to match: %s", event)

// 		if len(matches) != 1 {
// 			matchingPatterns := make([]string, len(matches))
// 			for i, match := range matches {
// 				matchingPatterns[i] = string(patterns[match])
// 			}
// 			t.Logf("Matching Patterns: %v", matchingPatterns)
// 			t.Logf("Event: %s", event)
// 		}
// 	}

// 	// Generate 10 events that should not match
// 	nonMatchingEvents := make([]string, 10)
// 	for i := 0; i < 10; i++ {
// 		event := map[string]interface{}{
// 			"field_0": generateRandomString(10),
// 			"field_1": generateRandomString(10),
// 		}
// 		eventJSON, err := json.Marshal(event)
// 		assert.NoError(t, err)
// 		nonMatchingEvents[i] = string(eventJSON)
// 	}

// 	// Test non-matching events
// 	for _, event := range nonMatchingEvents {
// 		matches, err := matcher.matchesForJSONEvent([]byte(event))
// 		assert.NoError(t, err)
// 		assert.Empty(t, matches, "Expected event not to match: %s", event)

// 		if len(matches) != 0 {
// 			matchingPatterns := make([]string, len(matches))
// 			for i, match := range matches {
// 				matchingPatterns[i] = string(patterns[match])
// 			}
// 			t.Logf("Matching Patterns: %v", matchingPatterns)
// 			t.Logf("Event: %s", event)
// 		}
// 	}
// }

func BenchmarkMatcherFromPatterns(b *testing.B) {
	patternsJSON := generatePatterns(100, []int{1000, 100})
	patterns := make(map[X]string)

	for i, pattern := range patternsJSON {
		patterns[X(fmt.Sprintf("pattern_%d", i))] = pattern
	}

	start := time.Now()
	_, err := MatcherFromPatterns(patterns)
	if err != nil {
		b.Fatalf("Error building matcher: %v", err)
	}
	b.Logf("Time to build matcher: %v", time.Since(start))

	start = time.Now()
	old := newCoreMatcher()
	for x, pattern := range patterns {
		old.addPattern(x, pattern)
	}
	b.Logf("Time to add patterns to old matcher: %v", time.Since(start))
}

func visualizeTrie(trie *trieNode) string {
	var buf bytes.Buffer
	visualizeTrieNode(&buf, trie, 0)
	return buf.String()
}

func visualizeTrieNode(buf *bytes.Buffer, node *trieNode, depth int) {
	indent := strings.Repeat("    ", depth)

	if node.isEnd {
		buf.WriteString(fmt.Sprintf("%s(end) %v\n", indent, node.memberOfPatterns))
	}

	// if len(node.vTypes) > 0 {
	// 	buf.WriteString(fmt.Sprintf("%sValue Types: %v\n", indent, node.vTypes))
	// }

	for ch, child := range node.children {
		if ch >= 32 && ch <= 126 { // printable ASCII
			buf.WriteString(fmt.Sprintf("%s'%c' -> %v\n", indent, ch, child.memberOfPatterns))
		} else {
			buf.WriteString(fmt.Sprintf("%s0x%02X -> %v\n", indent, ch, child.memberOfPatterns))
		}
		visualizeTrieNode(buf, child, depth+1)
	}

	if len(node.transition) > 0 {
		buf.WriteString(fmt.Sprintf("%sTransitions:\n", indent))
		for path, nextNode := range node.transition {
			buf.WriteString(fmt.Sprintf("%s  %s -> \n", indent, path))
			visualizePathTrieRecursive(buf, nextNode, depth+2)
		}
	}
}

func visualizePathTrie(trie *pathTrie) string {
	var buf bytes.Buffer
	visualizePathTrieRecursive(&buf, trie, 0)
	return buf.String()
}

func visualizePathTrieRecursive(buf *bytes.Buffer, trie *pathTrie, depth int) {
	indent := strings.Repeat("  ", depth)
	buf.WriteString(fmt.Sprintf("%sPath: %s\n", indent, trie.path))
	buf.WriteString(fmt.Sprintf("%sNode:\n", indent))
	visualizeTrieNode(buf, trie.node, depth+1)

	for path, nextTrie := range trie.node.transition {
		buf.WriteString(fmt.Sprintf("%sTransition to field: %s\n", indent, path))
		visualizePathTrieRecursive(buf, nextTrie, depth+1)
	}
}

func visualizeTriesAsMermaid(tries map[string]*pathTrie) string {
	var buf bytes.Buffer
	buf.WriteString("graph TD\n")
	visitedNodes := make(map[uintptr]string)
	visitedLinks := make(map[string]bool)

	// Create a root node to connect all tries
	rootId := "root"
	buf.WriteString(fmt.Sprintf("    %s[Root]\n", rootId))

	for field, trie := range tries {
		trieId := fmt.Sprintf("%p", trie)
		linkKey := fmt.Sprintf("%s-->%s", rootId, trieId)
		if !visitedLinks[linkKey] {
			buf.WriteString(fmt.Sprintf("    %s --> %s[%s]\n", rootId, trieId, field))
			visitedLinks[linkKey] = true
		}
		visualizePathTrieAsMermaidRecursive(&buf, trie, trieId, 0, visitedNodes, visitedLinks)
	}

	return buf.String()
}

func visualizePathTrieAsMermaid(buf *bytes.Buffer, trie *pathTrie, parentId string) string {
	var localBuf bytes.Buffer
	localBuf.WriteString("graph TD\n")
	visitedNodes := make(map[uintptr]string)
	visitedLinks := make(map[string]bool)
	visualizePathTrieAsMermaidRecursive(&localBuf, trie, parentId, 0, visitedNodes, visitedLinks)
	return localBuf.String()
}

func visualizePathTrieAsMermaidRecursive(buf *bytes.Buffer, trie *pathTrie, parentId string, depth int, visitedNodes map[uintptr]string, visitedLinks map[string]bool) {
	currentPtr := uintptr(unsafe.Pointer(trie))
	currentId, exists := visitedNodes[currentPtr]
	if !exists {
		currentId = fmt.Sprintf("%p", trie)
		visitedNodes[currentPtr] = currentId
		buf.WriteString(fmt.Sprintf("    %s[%s<br />%p]\n", currentId, trie.path, trie))
	}

	visualizeTrieNodeAsMermaid(buf, trie.node, &currentId, depth+1, visitedNodes, visitedLinks)

	for path, nextTrie := range trie.node.transition {
		nextPtr := uintptr(unsafe.Pointer(nextTrie))
		nextId, exists := visitedNodes[nextPtr]
		if !exists {
			nextId = fmt.Sprintf("%p", nextTrie)
			visitedNodes[nextPtr] = nextId
			buf.WriteString(fmt.Sprintf("    %s[%s<br />%p]\n", nextId, path, nextTrie))
		}

		linkKey := fmt.Sprintf("%s-->%s", currentId, nextId)
		if !visitedLinks[linkKey] {
			buf.WriteString(fmt.Sprintf("    %s --> %s\n", currentId, nextId))
			visitedLinks[linkKey] = true
		}

		visualizePathTrieAsMermaidRecursive(buf, nextTrie, currentId, depth+1, visitedNodes, visitedLinks)
	}
}

func visualizeTrieNodeAsMermaid(buf *bytes.Buffer, node *trieNode, parentId *string, depth int, visitedNodes map[uintptr]string, visitedLinks map[string]bool) {
	currentPtr := uintptr(unsafe.Pointer(node))
	currentId, exists := visitedNodes[currentPtr]
	if !exists {
		currentId = fmt.Sprintf("%p", node)
		visitedNodes[currentPtr] = currentId
	}

	// Handle early end state (isEnd) and wildcard (isWildcard)
	if node.isEnd || node.isWildcard {
		endId := fmt.Sprintf("%s_end", currentId)
		endLabel := "END"
		if node.isWildcard {
			endLabel = "WILDCARD"
		}
		patterns := make([]string, 0, len(node.memberOfPatterns))
		for pattern := range node.memberOfPatterns {
			patterns = append(patterns, pattern.(string))
		}
		patternsStr := strings.Join(patterns, ", ")
		buf.WriteString(fmt.Sprintf("    %s((%s<br />%p<br />%s))\n", endId, endLabel, node, patternsStr))

		endLinkKey := fmt.Sprintf("%s-->%s", *parentId, endId)
		if !visitedLinks[endLinkKey] {
			buf.WriteString(fmt.Sprintf("    %s --> %s\n", *parentId, endId))
			visitedLinks[endLinkKey] = true
		}
	}

	for ch, child := range node.children {
		childPtr := uintptr(unsafe.Pointer(child))
		childId, exists := visitedNodes[childPtr]
		if !exists {
			childId = fmt.Sprintf("%p", child)
			visitedNodes[childPtr] = childId
			if ch == '"' {
				buf.WriteString(fmt.Sprintf("    %s[QUOTE<br />%p]\n", childId, child))
			} else {
				buf.WriteString(fmt.Sprintf("    %s[%c<br />%p]\n", childId, ch, child))
			}
		}

		linkKey := fmt.Sprintf("%s-->%s", *parentId, childId)
		if !visitedLinks[linkKey] {
			buf.WriteString(fmt.Sprintf("    %s --> %s\n", *parentId, childId))
			visitedLinks[linkKey] = true
		}

		visualizeTrieNodeAsMermaid(buf, child, &childId, depth+1, visitedNodes, visitedLinks)
	}

	for path, nextTrie := range node.transition {
		nextPtr := uintptr(unsafe.Pointer(nextTrie))
		nextId, exists := visitedNodes[nextPtr]
		if !exists {
			nextId = fmt.Sprintf("%p", nextTrie)
			visitedNodes[nextPtr] = nextId
			buf.WriteString(fmt.Sprintf("    %s[%s<br />%p]\n", nextId, path, nextTrie))
		}

		linkKey := fmt.Sprintf("%s-->%s", *parentId, nextId)
		if !visitedLinks[linkKey] {
			buf.WriteString(fmt.Sprintf("    %s --> %s\n", *parentId, nextId))
			visitedLinks[linkKey] = true
		}

		visualizePathTrieAsMermaidRecursive(buf, nextTrie, *parentId, depth+1, visitedNodes, visitedLinks)
	}
}
