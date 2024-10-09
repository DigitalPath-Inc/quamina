package quamina

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

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
	// patterns[X("pattern_2")] = `{"field_0":["foo", "bar"], "field_1":["asdf", "qwer"]}`
	// patterns[X("pattern_3")] = `{"field_0":["foo", "baz"], "field_1":["asdf", "zxcv"]}`
	patterns[X("pattern_0")] = `{"field_0":[{"prefix": "foo"}], "field_1":[{"prefix": "bar"}]}`

	fmt.Printf("Patterns: %v\n", patterns)

	start := time.Now()
	fields := make(map[string]struct{})
	trie, err := trieFromPatterns(patterns, &fields)
	if err != nil {
		t.Fatalf("Error building trie: %v", err)
	}
	t.Logf("Time to build trie: %v", time.Since(start))
	t.Logf("Trie:\n%v", visualizePathTrie(trie))
}

func TestMatcherFromPatterns(t *testing.T) {
	testCases := []struct {
		name     string
		patterns map[X]string
		events   [][]byte
		expected [][]X
	}{
		// {
		// 	name: "Multiple events and matches",
		// 	patterns: map[X]string{
		// 		X("pattern_0"): `{"field_0":["foo", "bar"], "field_1":["asdf", "qwer"]}`,
		// 		X("pattern_1"): `{"field_0":["baz", "qux"], "field_1":["asdf", "zxcv"]}`,
		// 	},
		// 	events: [][]byte{
		// 		[]byte(`{"field_0": "foo", "field_1": "asdf"}`),
		// 		[]byte(`{"field_0": "baz", "field_1": "asdf"}`),
		// 		[]byte(`{"field_0": "foo", "field_1": "zxcv"}`),
		// 	},
		// 	expected: [][]X{
		// 		{X("pattern_0")},
		// 		{X("pattern_1")},
		// 		{},
		// 	},
		// },
		{
			name: "Different types of patterns",
			patterns: map[X]string{
				X("pattern_0"): `{"field_0":[{"prefix": "foo"}], "field_1":["foo", {"prefix": "bar"}, "baz"]}`,
				// X("pattern_1"): `{"field_0":[{"anything-but": ["a", "b"]}]}`,
				// X("pattern_1"): `{"field_0":["foo", "bar"]}`,
				// X("pattern_1"): `{"field_0":[{"anything-but": ["baz", "qux"]}], "field_1":[{"wildcard": "a*f"}]}`,
				// X("pattern_2"): `{"field_0":[{"equals-ignore-case": "Hello"}], "field_1":[{"wildcard": "*.jpg"}]}`,
			},
			events: [][]byte{
				[]byte(`{"field_0": "foo", "field_1": "asdf"}`),
				// []byte(`{"field_0": "barcode", "field_1": "present"}`),
				// []byte(`{"field_0": "hello", "field_1": "image.jpg"}`),
				// []byte(`{"field_0": "baz", "field_1": "abcf"}`),
			},
			expected: [][]X{
				{X("pattern_0"), X("pattern_1")},
				// {X("pattern_0")},
				// {X("pattern_2")},
				// {X("pattern_1")},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			matcher, err := MatcherFromPatterns(tc.patterns)
			assert.NoError(t, err)

			oldMatcher := newCoreMatcher()
			for x, pattern := range tc.patterns {
				oldMatcher.addPattern(x, pattern)
			}

			t.Logf("Matcher: %v", unravelMatcher(matcher))
			t.Logf("Old Matcher: %v", unravelMatcher(oldMatcher))

			// for i, event := range tc.events {
			// 	matches, err := matcher.matchesForJSONEvent(event)
			// 	assert.NoError(t, err)
			// 	oldMatches, err := oldMatcher.matchesForJSONEvent(event)
			// 	assert.NoError(t, err)
			// 	assert.Equal(t, tc.expected[i], matches)
			// 	if !assert.Equal(t, oldMatches, matches) {
			// 		t.Logf("Patterns: %v, Event: %v, Matches: %v", tc.patterns, string(event), matches)
			// 		t.Logf("Matcher: %v", unravelMatcher(matcher))
			// 		t.Logf("Old Matcher: %v", unravelMatcher(oldMatcher))
			// 	}
			// }
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
	patternsJSON := generatePatterns(10000, []int{1000})
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
