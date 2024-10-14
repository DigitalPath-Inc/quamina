package quamina

import (
	"fmt"
	"sort"
	"time"
)

type pathTrie struct {
	path               string
	node               *trieNode
	hasNumbers         bool
	isNondeterministic bool
}

type trieNode struct {
	children         map[byte]*trieNode
	isEnd            bool
	isWildcard       bool
	memberOfPatterns map[X]struct{}
	transition       map[string]*pathTrie
	hash             uint64
}

var visualizer = newMermaidTrieVisualizer()

func (t *pathTrie) generateHash() uint64 {
	if t.node == nil {
		return 0
	}

	var hash uint64 = 5381 // Initial value for DJB2 hash algorithm

	// Hash the path
	for _, ch := range t.path {
		hash = ((hash << 5) + hash) + uint64(ch)
	}

	// Hash the node
	nodeHash := t.node.generateHash()
	hash = ((hash << 5) + hash) + nodeHash

	t.node.hash = hash // Store the hash in the node
	return hash
}

func (t *trieNode) generateHash() uint64 {
	if t.hash != 0 {
		return t.hash // Return cached hash if available
	}

	var hash uint64 = 5381 // Initial value for DJB2 hash algorithm

	// Hash children
	childKeys := make([]byte, 0, len(t.children))
	for ch := range t.children {
		childKeys = append(childKeys, ch)
	}
	if len(childKeys) < 20 {
		// Use insertion sort for small slices
		for i := 1; i < len(childKeys); i++ {
			for j := i; j > 0 && childKeys[j] < childKeys[j-1]; j-- {
				childKeys[j], childKeys[j-1] = childKeys[j-1], childKeys[j]
			}
		}
	} else {
		sort.Slice(childKeys, func(i, j int) bool { return childKeys[i] < childKeys[j] })
	}

	for _, ch := range childKeys {
		child := t.children[ch]
		childHash := child.generateHash()
		hash = ((hash << 5) + hash) + uint64(ch)
		hash = ((hash << 5) + hash) + childHash
	}

	// Hash isEnd and isWildcard
	if t.isEnd {
		hash = ((hash << 5) + hash) + 1
	}
	if t.isWildcard {
		hash = ((hash << 5) + hash) + 1
	}

	// Hash memberOfPatterns
	patternKeys := make([]string, 0, len(t.memberOfPatterns))
	for x := range t.memberOfPatterns {
		patternKeys = append(patternKeys, x.(string))
	}
	if len(patternKeys) < 20 {
		// Use insertion sort for small slices
		for i := 1; i < len(patternKeys); i++ {
			for j := i; j > 0 && patternKeys[j] < patternKeys[j-1]; j-- {
				patternKeys[j], patternKeys[j-1] = patternKeys[j-1], patternKeys[j]
			}
		}
	} else {
		sort.Strings(patternKeys)
	}

	for _, x := range patternKeys {
		for _, ch := range x {
			hash = ((hash << 5) + hash) + uint64(ch)
		}
	}

	// Hash transitions
	transitionKeys := make([]string, 0, len(t.transition))
	for path := range t.transition {
		transitionKeys = append(transitionKeys, path)
	}
	if len(transitionKeys) < 20 {
		// Use insertion sort for small slices
		for i := 1; i < len(transitionKeys); i++ {
			for j := i; j > 0 && transitionKeys[j] < transitionKeys[j-1]; j-- {
				transitionKeys[j], transitionKeys[j-1] = transitionKeys[j-1], transitionKeys[j]
			}
		}
	} else {
		sort.Strings(transitionKeys)
	}

	for _, path := range transitionKeys {
		pathTrie := t.transition[path]
		for _, ch := range path {
			hash = ((hash << 5) + hash) + uint64(ch)
		}
		hash = ((hash << 5) + hash) + pathTrie.generateHash()
	}

	t.hash = hash // Cache the computed hash
	return hash
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

func trieFromPatterns(patterns map[X]string, allFields *map[string]struct{}) (map[string]*pathTrie, error) {
	// start := time.Now()
	root := make(map[string]*pathTrie)

	patternJSONTime := time.Duration(0)
	buildTrieTime := time.Duration(0)

	for x, patternJSON := range patterns {
		patternJSONStart := time.Now()
		fields, err := patternFromJSON([]byte(patternJSON))
		patternJSONTime += time.Since(patternJSONStart)
		if err != nil {
			return nil, err
		}

		buildTrieStart := time.Now()
		err = buildTrie(root, fields, x)
		// visualizer.reset()
		// fmt.Printf("New trie:\n%v\n", visualizer.visualize(root))
		buildTrieTime += time.Since(buildTrieStart)
		if err != nil {
			return nil, err
		}

		for _, field := range fields {
			(*allFields)[field.path] = struct{}{}
		}
	}

	for _, trie := range root {
		trie.generateHash()
	}

	// totalDuration := time.Since(start)
	// fmt.Printf("trieFromPatterns total execution time: %v\n", totalDuration)
	// fmt.Printf("patternFromJSON total time: %v\n", patternJSONTime)
	// fmt.Printf("buildTrie total time: %v\n", buildTrieTime)
	return root, nil
}

func buildTrie(root map[string]*pathTrie, fields []*patternField, x X) error {
	if len(fields) == 0 {
		return nil
	}

	currentField := fields[0]
	remainingFields := fields[1:]

	if _, exists := root[currentField.path]; !exists {
		root[currentField.path] = newPathTrie(currentField.path)
	}

	trie := root[currentField.path]

	for _, val := range currentField.vals {
		err := insertValue(trie, val, remainingFields, x)
		if err != nil {
			return err
		}
	}

	return nil
}

func insertValue(trie *pathTrie, val typedVal, remainingFields []*patternField, x X) error {
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
	case monocaseType:
		err = insertMonocaseValue(trie, val.val, remainingFields, x)
	case wildcardType:
		err = insertWildcardValue(trie, val.val, remainingFields, x)
	default:
		return fmt.Errorf("unknown value type: %v", val.vType)
	}
	if err != nil {
		return err
	}
	return nil
}

func handleTransition(trie *pathTrie, node *trieNode, remainingFields []*patternField, x X) error {
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
		return buildTrie(node.transition, remainingFields, x)
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

	err := handleTransition(trie, node, remainingFields, x)
	if err != nil {
		return err
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

	err := handleTransition(trie, node, remainingFields, x)
	if err != nil {
		return err
	}

	return nil
}

// insertMonocaseValue uses the same logic as insertStringValue but handles case folding. It basically generates two paths for each rune in the value
// and adds them to the trie. We first generate all the permutations of the value, then add each as a trie
func insertMonocaseValue(trie *pathTrie, value string, remainingFields []*patternField, x X) error {
	nodes := []*trieNode{trie.node}
	children := make(map[byte]*trieNode)
	for _, r := range value {
		if alt, exists := caseFoldingPairs[r]; exists {
			if _, nodeExists := children[byte(alt)]; !nodeExists {
				children[byte(alt)] = newTrie()
			}
		}
		if _, exists := children[byte(r)]; !exists {
			children[byte(r)] = newTrie()
		}

		newNodes := make([]*trieNode, 0, len(children))
		for _, n := range nodes {
			if n.children == nil {
				n.children = make(map[byte]*trieNode)
			}
			for ch, child := range children {
				if _, exists := n.children[ch]; !exists {
					n.children[ch] = child
				}
				newNodes = append(newNodes, n.children[ch])
			}
		}

		nodes = newNodes
		children = make(map[byte]*trieNode)
	}

	for _, node := range nodes {
		err := handleTransition(trie, node, remainingFields, x)
		if err != nil {
			return err
		}
	}

	return nil
}

func insertWildcardValue(trie *pathTrie, value string, remainingFields []*patternField, x X) error {
	node := trie.node

	var stop int // Stop one short to skip the closing quote if the * is at the end
	if value[len(value)-1] == '*' && value[len(value)-2] != '\\' {
		stop = len(value) - 1
	} else {
		stop = len(value)
		trie.isNondeterministic = true
	}

	for i := 0; i < stop; i++ {
		ch := value[i]
		if node.children == nil {
			node.children = make(map[byte]*trieNode)
		}
		if _, exists := node.children[ch]; !exists {
			node.children[ch] = newTrie()
		}
		node = node.children[ch]

		// If the next character is a * set isWildcard to true and advance to the character after
		if i+1 < stop && value[i+1] == '*' {
			node.isWildcard = true
			i++
		}
	}

	err := handleTransition(trie, node, remainingFields, x)
	if err != nil {
		return err
	}

	return nil
}

func convertTrieToCoreMatcher(cm *coreMatcher, root map[string]*pathTrie) error {
	fields := cm.fields()
	freshFields := &coreFields{
		state:        fields.state,
		segmentsTree: fields.segmentsTree.copy(),
		nfaMeta:      fields.nfaMeta,
	}

	freshState := freshFields.state.fields()
	freshState.transitions = make(map[string]*valueMatcher)

	for path, trie := range root {
		vm := newValueMatcher()
		err := convertPathTrieToValueMatcher(trie, vm)
		if err != nil {
			return err
		}
		freshState.transitions[path] = vm
		vm.fields().isNondeterministic = trie.isNondeterministic
		vm.fields().hasNumbers = trie.hasNumbers
	}

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

	if node.isEnd || (node.isWildcard && len(node.children) == 0) {
		createEndState(node, &states)
	} else if len(node.transition) > 0 {
		createTransitionState(node, &states)
	}

	for ch, child := range node.children {
		nextState := getOrCreateState(child)
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

	if len(node.children) == 0 {
		fmt.Println("Children: ", node.children)
		fmt.Println("Transitions: ", node.transition)
		fmt.Println("IsEnd: ", node.isEnd)
		fmt.Println("IsWildcard: ", node.isWildcard)
		fmt.Println("MemberOfPatterns: ", node.memberOfPatterns)
		fmt.Printf("Bytes: %v\n", bytes)
		fmt.Printf("Value Terminator: %v\n", valueTerminator)
		fmt.Printf("Steps: %v\n", steps)
	}
	table := makeSmallTable(nil, bytes, steps)

	if node.isWildcard && len(node.children) > 0 {
		if table.epsilon == nil {
			table.epsilon = make([]*faState, 1)
		}
		epsilon := &faState{table: table}
		table.epsilon[0] = epsilon
	}

	return table, nil
}

type stateCache map[uint64]*faNext

var globalStateCache = make(stateCache)

func createEndState(node *trieNode, states *map[byte]*faNext) {
	key := getStateKey(node)
	if cachedState, exists := globalStateCache[key]; exists {
		(*states)[valueTerminator] = cachedState
		return
	}

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

	globalStateCache[key] = &faNext{states: []*faState{endState}}
	(*states)[valueTerminator] = globalStateCache[key]
	// (*states)[byte(byteCeiling)] = nil
}

func createTransitionState(node *trieNode, states *map[byte]*faNext) {
	key := getStateKey(node)
	if cachedState, exists := globalStateCache[key]; exists {
		(*states)[valueTerminator] = cachedState
		return
	}

	transitionState, _ := handleTransitions(node.transition)
	globalStateCache[key] = &faNext{states: []*faState{transitionState}}
	(*states)[valueTerminator] = globalStateCache[key]
}

// func createWildcardState(node *trieNode, nextState *faState) error {
// 	fmt.Printf("Wildcard called with %v children\n", len(node.children))
// 	if len(node.children) != 0 {
// 		table, err := buildSmallTableFromTrie(node)
// 		if err != nil {
// 			return err
// 		}
// 		if table.epsilon == nil {
// 			table.epsilon = make([]*faState, 1)
// 		}
// 		table.epsilon = append(table.epsilon, &faState{table: table})

// 		nextState.table = table
// 	}

// 	for x := range node.memberOfPatterns {
// 		fm := newFieldMatcher()
// 		fm.addMatch(x)
// 		nextState.fieldTransitions = append(nextState.fieldTransitions, fm)
// 	}

// 	if len(node.transition) > 0 {
// 		transitionState, err := handleTransitions(node.transition)
// 		if err != nil {
// 			return err
// 		}
// 		nextState.fieldTransitions = append(nextState.fieldTransitions, transitionState.fieldTransitions...)
// 	}

// 	return nil
// }

func getStateKey(node *trieNode) uint64 {
	return node.hash
	// var key strings.Builder

	// // Add basic properties
	// key.WriteString(fmt.Sprintf("end:%v|wildcard:%v|", node.isEnd, node.isWildcard))

	// // Add patterns
	// patterns := make([]string, 0, len(node.memberOfPatterns))
	// for x := range node.memberOfPatterns {
	// 	patterns = append(patterns, x.(string))
	// }
	// sort.Strings(patterns)
	// key.WriteString(fmt.Sprintf("patterns:%v|", patterns))

	// // Add children
	// childKeys := make([]string, 0, len(node.children))
	// for ch, child := range node.children {
	// 	childKey := fmt.Sprintf("%d:%s", ch, getStateKey(child))
	// 	childKeys = append(childKeys, childKey)
	// }
	// sort.Strings(childKeys)
	// key.WriteString(fmt.Sprintf("children:%v|", childKeys))

	// // Add transitions
	// transitions := make([]string, 0, len(node.transition))
	// for t, pathTrie := range node.transition {
	// 	transitionKey := fmt.Sprintf("%s:%s", t, getStateKey(pathTrie.node))
	// 	transitions = append(transitions, transitionKey)
	// }
	// sort.Strings(transitions)
	// key.WriteString(fmt.Sprintf("transitions:%v", transitions))

	// return key.String()
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

func getOrCreateState(node *trieNode) *faState {
	key := getStateKey(node)
	if cachedState, exists := globalStateCache[key]; exists {
		return cachedState.states[0]
	}

	newState := &faState{
		table: newSmallTable(),
	}

	// if node.isWildcard {
	// 	createWildcardState(node, newState)
	// } else {
	childTable, _ := buildSmallTableFromTrie(node)
	newState.table = childTable
	// }

	globalStateCache[key] = &faNext{states: []*faState{newState}}
	return newState
}
