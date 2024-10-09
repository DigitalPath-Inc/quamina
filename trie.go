package quamina

import (
	"fmt"
	"sort"
	"time"
)

type pathTrie struct {
	path string
	node *trieNode
}

type trieNode struct {
	children         map[byte]*trieNode
	isEnd            bool
	isWildcard       bool
	memberOfPatterns map[X]struct{}
	transition       map[string]*pathTrie
}

func newPathTrie(path string) *pathTrie {
	return &pathTrie{
		path: path,
		node: newTrie(),
	}
}

func newTrie() *trieNode {
	return &trieNode{
		children:         nil, // make(map[byte]*trieNode),
		memberOfPatterns: nil, // make(map[X]bool),
		transition:       nil, // make(map[string]*pathTrie),
	}
}

func (t *trieNode) insert(value []byte, x X) {
	node := t
	for _, ch := range value {
		if _, exists := node.children[ch]; !exists {
			node.children[ch] = newTrie()
		}
		node = node.children[ch]
	}
	node.isEnd = true
	node.memberOfPatterns[x] = struct{}{}
}

func MatcherFromPatterns(patterns map[X]string) (*coreMatcher, error) {
	// start := time.Now()
	fields := make(map[string]struct{})
	root, err := trieFromPatterns(patterns, &fields)
	if err != nil {
		return nil, err
	}
	// trieTime := time.Since(start)
	// fmt.Printf("trieFromPatterns Time: %v\n", trieTime)

	// start = time.Now()
	cm := newCoreMatcher()
	convertTrieToCoreMatcher(cm, root)
	// convertTime := time.Since(start)
	// fmt.Printf("convertTrieToCoreMatcher Time: %v\n", convertTime)

	segmentsTree := newSegmentsIndex()
	for field := range fields {
		segmentsTree.add(field)
	}
	cm.fields().segmentsTree = segmentsTree

	return cm, nil
}

func trieFromPatterns(patterns map[X]string, allFields *map[string]struct{}) (*pathTrie, error) {
	// start := time.Now()
	var root *pathTrie

	patternJSONTime := time.Duration(0)
	buildTrieTime := time.Duration(0)

	for x, patternJSON := range patterns {
		patternJSONStart := time.Now()
		fields, err := patternFromJSON([]byte(patternJSON))
		patternJSONTime += time.Since(patternJSONStart)
		if err != nil {
			return nil, err
		}

		if root == nil {
			root = newPathTrie(fields[0].path)
		}

		buildTrieStart := time.Now()
		err = buildTrie(root, fields, x)
		buildTrieTime += time.Since(buildTrieStart)
		if err != nil {
			return nil, err
		}

		for _, field := range fields {
			(*allFields)[field.path] = struct{}{}
		}
	}

	// totalDuration := time.Since(start)
	// fmt.Printf("trieFromPatterns total execution time: %v\n", totalDuration)
	// fmt.Printf("patternFromJSON total time: %v\n", patternJSONTime)
	// fmt.Printf("buildTrie total time: %v\n", buildTrieTime)
	return root, nil
}

func buildTrie(trie *pathTrie, fields []*patternField, x X) error {
	if len(fields) == 0 {
		return nil
	}

	currentField := fields[0]
	remainingFields := fields[1:]

	for _, val := range currentField.vals {
		var err error
		switch val.vType {
		case stringType, literalType, numberType:
			err = insertStringValue(trie, val.val, remainingFields, x)
		// case existsTrueType, existsFalseType:
		// 	err = insertExistsValue(trie, val.vType == existsTrueType, remainingFields, x)
		// case shellStyleType:
		// 	err = insertShellStyleValue(trie, val.val, remainingFields, x)
		// case anythingButType:
		// 	err = insertAnythingButValue(trie, val.list, remainingFields, x)
		case prefixType:
			err = insertPrefixValue(trie, val.val, remainingFields, x)
		// case monocaseType:
		// 	err = insertMonocaseValue(trie, val.val, remainingFields, x)
		// case wildcardType:
		// 	err = insertWildcardValue(trie, val.val, remainingFields, x)
		default:
			return fmt.Errorf("unknown value type: %v", val.vType)
		}
		if err != nil {
			return err
		}
	}

	return nil
}

func insertStringValue(trie *pathTrie, value string, remainingFields []*patternField, x X) error {
	node := trie.node
	for _, ch := range []byte(value) {
		if node.children == nil {
			node.children = make(map[byte]*trieNode)
		}
		if _, exists := node.children[ch]; !exists {
			node.children[ch] = newTrie()
		}
		node = node.children[ch]
	}

	if len(remainingFields) == 0 {
		node.isEnd = true
		if node.memberOfPatterns == nil {
			node.memberOfPatterns = make(map[X]struct{})
		}
		node.memberOfPatterns[x] = struct{}{}
	} else {
		if node.transition == nil {
			node.transition = make(map[string]*pathTrie)
		}
		if _, exists := node.transition[remainingFields[0].path]; !exists {
			nextTrie := newPathTrie(remainingFields[0].path)
			node.transition[remainingFields[0].path] = nextTrie
			return buildTrie(nextTrie, remainingFields, x)
		} else {
			return buildTrie(node.transition[remainingFields[0].path], remainingFields, x)
		}
	}

	return nil
}

func insertPrefixValue(trie *pathTrie, value string, remainingFields []*patternField, x X) error {
	node := trie.node
	for i := 0; i < len(value)-1; i++ { // Stop one short to skip the closing quote
		ch := value[i]
		if node.children == nil {
			node.children = make(map[byte]*trieNode)
		}
		if _, exists := node.children[ch]; !exists {
			node.children[ch] = newTrie()
		}
		node = node.children[ch]
	}

	// Mark the last node as a prefix end
	node.isWildcard = true

	if len(remainingFields) == 0 {
		if node.memberOfPatterns == nil {
			node.memberOfPatterns = make(map[X]struct{})
		}
		node.memberOfPatterns[x] = struct{}{}
	} else {
		if node.transition == nil {
			node.transition = make(map[string]*pathTrie)
		}
		if _, exists := node.transition[remainingFields[0].path]; !exists {
			nextTrie := newPathTrie(remainingFields[0].path)
			node.transition[remainingFields[0].path] = nextTrie
			return buildTrie(nextTrie, remainingFields, x)
		} else {
			return buildTrie(node.transition[remainingFields[0].path], remainingFields, x)
		}
	}

	return nil
}

func convertTrieToCoreMatcher(cm *coreMatcher, root *pathTrie) error {
	fields := cm.fields()
	freshFields := &coreFields{
		state:        fields.state,
		segmentsTree: fields.segmentsTree.copy(),
		nfaMeta:      fields.nfaMeta,
	}

	freshState := freshFields.state.fields()
	freshState.transitions = make(map[string]*valueMatcher)

	vm := newValueMatcher()
	err := convertPathTrieToValueMatcher(root, vm)
	if err != nil {
		return err
	}
	freshState.transitions[root.path] = vm

	// fmt.Printf("Root path: %s\n", root.path)
	// fmt.Printf("ValueMatcher for root: %+v\n", vm)

	freshFields.state.update(freshState)
	cm.updateable.Store(freshFields)

	// fmt.Printf("CoreMatcher after conversion:\n%+v\n", cm)
	return nil
}

func convertPathTrieToValueMatcher(pt *pathTrie, vm *valueMatcher) error {
	err := convertTrieNodeToValueMatcher(pt.node, vm)
	if err != nil {
		return err
	}

	return nil
}

func convertTrieNodeToValueMatcher(node *trieNode, vm *valueMatcher) error {
	fields := vm.getFieldsForUpdate()

	table, err := buildSmallTableFromTrie(node)
	if err != nil {
		return err
	}
	fields.startTable = table

	vm.update(fields)
	return nil
}

func buildSmallTableFromTrie(node *trieNode) (*smallTable, error) {
	size := len(node.children) + len(node.transition)
	states := make(map[byte]*faNext, size)

	if node.isEnd || (node.isWildcard && len(node.memberOfPatterns) > 0) {
		endState := &faState{
			table:            newSmallTable(),
			fieldTransitions: make([]*fieldMatcher, 0, len(node.memberOfPatterns)),
		}

		// Handle pattern matches
		for x := range node.memberOfPatterns {
			fm := newFieldMatcher()
			fm.addMatch(x)
			endState.fieldTransitions = append(endState.fieldTransitions, fm)
		}
		states[valueTerminator] = &faNext{states: []*faState{endState}}
	}

	// Handle transitions
	if len(node.transition) > 0 {
		transitionState, err := handleTransitions(node.transition)
		if err != nil {
			return nil, err
		}
		states[valueTerminator] = &faNext{states: []*faState{transitionState}}
	}

	for ch, child := range node.children {
		fmt.Printf("Child: %v\n", ch)
		fmt.Printf("Child attrs: %+v\n", child)

		nextState := &faState{
			table: newSmallTable(),
		}

		// Handle the wildcard scenario without adding a new state
		if child.isWildcard {
			for x := range child.memberOfPatterns {
				fm := newFieldMatcher()
				fm.addMatch(x)
				nextState.fieldTransitions = append(nextState.fieldTransitions, fm)
			}

			if len(child.transition) > 0 {
				transitionState, err := handleTransitions(child.transition)
				if err != nil {
					return nil, err
				}
				nextState.fieldTransitions = append(nextState.fieldTransitions, transitionState.fieldTransitions...)
			}
		} else {
			childTable, err := buildSmallTableFromTrie(child)
			if err != nil {
				return nil, err
			}
			nextState.table = childTable
		}

		states[ch] = &faNext{states: []*faState{nextState}}
	}

	// Convert the states map to a sorted slice of byte and faNext pairs
	bytes := make([]byte, 0, len(states))
	steps := make([]*faNext, 0, len(states))
	for ch := range states {
		bytes = append(bytes, ch)
	}
	sort.Slice(bytes, func(i, j int) bool {
		return bytes[i] < bytes[j]
	})

	for _, ch := range bytes {
		steps = append(steps, states[ch])
	}

	if len(bytes) == 0 {
		fmt.Println("Children: ", node.children)
		fmt.Println("Transitions: ", node.transition)
		fmt.Println("IsEnd: ", node.isEnd)
		fmt.Println("IsWildcard: ", node.isWildcard)
		fmt.Println("MemberOfPatterns: ", node.memberOfPatterns)
	}
	return makeSmallTable(nil, bytes, steps), nil
}

func handleTransitions(transitions map[string]*pathTrie) (*faState, error) {
	transitionVMs := make(map[string]*valueMatcher)
	nextState := &faState{
		table:            newSmallTable(),
		fieldTransitions: make([]*fieldMatcher, 0),
	}

	for nextFieldPath, transitionNode := range transitions {
		nextVM := newValueMatcher()
		err := convertPathTrieToValueMatcher(transitionNode, nextVM)
		if err != nil {
			return nil, err
		}
		transitionVMs[nextFieldPath] = nextVM
	}
	fields := &fmFields{
		transitions: transitionVMs,
		existsTrue:  make(map[string]*fieldMatcher),
		existsFalse: make(map[string]*fieldMatcher),
	}
	fm := &fieldMatcher{}
	fm.updateable.Store(fields)

	nextState.fieldTransitions = append(nextState.fieldTransitions, fm)
	return nextState, nil
}

type valueMatcherPrinter struct {
	vm *valueMatcher
}

func (vmp *valueMatcherPrinter) labelTable(table *smallTable, label string) {
	// Implementation can be empty if you don't need to do anything here
}
