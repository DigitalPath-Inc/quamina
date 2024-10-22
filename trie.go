package quamina

import (
	"fmt"
	"sort"
)

// This file uses a different, faster approach to building the `coreMatcher` and subcomponents.
// We start out with a lighter-weight trie structure that is easier to convert to the coreMatcher.
// By creating this structure up front, we can generate a hash on each of the leaves and are able to re-use end states,
// which saves a ton of time on object creation. We also can do a direct insert without unpacking and repacking the smallTable,
// which is the bottleneck of the previous approach.

// A `pathTrie` represents a path in the trie with associated metadata. It could be thought of as a `fieldMatcher` equivalent.
type pathTrie struct {
	path               string
	node               *trieNode
	hasNumbers         bool
	isNondeterministic bool
}

// A `trieNode` represents a node in the trie, containing children, flags, and transitions. It could be thought of as a `valueMatcher` equivalent.
type trieNode struct {
	children         map[byte]*trieNode
	isEnd            bool
	isWildcard       bool
	memberOfPatterns map[X]struct{}
	transition       map[string]*pathTrie
	hash             uint64
}

// stateCache is a map of uint64 hashes to faNext pointers. It is used to cache faNext structures that have already been created.
// The globalStateCache is used to avoid duplicating faNext structures when converting the trie to a coreMatcher.
type stateCache map[uint64]*faNext

var globalStateCache = make(stateCache)

// generateHash recursively computes a hash for each leaf in a pathTrie. This key is later used in the conversion process to de-duplicate end states.
// Both the pathTrie and trieNode have a generateHash method because of the recursive nature of the trie.
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

func getStateKey(node *trieNode) uint64 {
	return node.hash
}

// newPathTrie creates a new pathTrie with a given path and a new trieNode.
func newPathTrie(path string) *pathTrie {
	return &pathTrie{
		path: path,
		node: newTrie(),
	}
}

// newTrie creates a new trieNode with a nil children map, nil memberOfPatterns map, and nil transition map.
// The values are not initialized due to the time cost of doing so.
func newTrie() *trieNode {
	return &trieNode{
		children:         nil, // make(map[byte]*trieNode),
		memberOfPatterns: nil, // make(map[X]bool),
		transition:       nil, // make(map[string]*pathTrie),
	}
}

// MatcherFromPatterns is the main function for building a coreMatcher from a set of patterns.
// It essentially just builds a trie from the patterns and then converts it to a coreMatcher.
func MatcherFromPatterns(patterns map[X]string) (*coreMatcher, error) {
	fields := make(map[string]struct{})
	root, err := trieFromPatterns(patterns, &fields)
	if err != nil {
		return nil, err
	}

	cm := newCoreMatcher()
	convertTrieToCoreMatcher(cm, root)

	segmentsTree := newSegmentsIndex()
	for field := range fields {
		segmentsTree.add(field)
	}
	cm.fields().segmentsTree = segmentsTree

	return cm, nil
}

// trieFromPatterns builds the trie from the patterns. It does so by first converting the pattern JSON to a patternField,
// then building the trie from the patternFields.
// `allFields` is a pointer to a map that is used to track all the fields in the trie.
func trieFromPatterns(patterns map[X]string, allFields *map[string]struct{}) (map[string]*pathTrie, error) {
	root := make(map[string]*pathTrie)

	for x, patternJSON := range patterns {
		fields, err := patternFromJSON([]byte(patternJSON))
		if err != nil {
			return nil, err
		}

		err = buildTrie(root, fields, x)
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

	return root, nil
}

// buildTrie is the core function for building the trie. It inserts the patternFields into the trie.
// It is called recursively to handle nested fields.
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

// insertValue determines the type of the value and inserts it into the trie accordingly.
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

// handleTrieTransition handles the transition from the current path to the next path.
// It sets the isEnd flag if there are no remaining fields and adds the pattern to the node's memberOfPatterns map.
// If there are remaining fields, it builds the trie for the remaining fields.
func handleTrieTransition(node *trieNode, remainingFields []*patternField, x X) error {
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

// insertStringValue inserts a string value into the trie.
// It iterates over each character in the string, creating new nodes as necessary, and then calls handleTrieTransition to handle the transition to the next path.
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

	err := handleTrieTransition(node, remainingFields, x)
	if err != nil {
		return err
	}

	return nil
}

// insertPrefixValue inserts a prefix value into the trie.
// It iterates over each character in the prefix, creating new nodes as necessary, and then calls handleTrieTransition to handle the transition to the next path.
// This is basically the same as `insertWildcardValue` but without the nondeterministic flag.
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

	// Add a "*" child to the last node
	if node.children == nil {
		node.children = make(map[byte]*trieNode)
	}
	if _, exists := node.children['*']; !exists {
		node.children['*'] = newTrie()
	}
	node = node.children['*']

	// Mark the last node as a prefix end
	node.isWildcard = true

	err := handleTrieTransition(node, remainingFields, x)
	if err != nil {
		return err
	}

	return nil
}

// insertMonocaseValue uses the same logic as insertStringValue but handles case folding. It basically sets up a primary path and then creates a secondary path for each case folding pair.
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
		err := handleTrieTransition(node, remainingFields, x)
		if err != nil {
			return err
		}
	}

	return nil
}

// insertWildcardValue inserts a wildcard value into the trie.
// It iterates over each character in the wildcard, creating new nodes as necessary, and then calls handleTrieTransition to handle the transition to the next path.
// The nondeterministic flag is set if the wildcard is not at the end of the value.
// This function does not currently handle escaping of the * character.
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

		if value[i] == '*' {
			node.isWildcard = true
		}
	}

	err := handleTrieTransition(node, remainingFields, x)
	if err != nil {
		return err
	}

	return nil
}

// convertTrieToCoreMatcher is the main function for converting the trie to a coreMatcher.
// It creates a fresh set of coreFields, then iterates over each path in the trie, converting each path to a valueMatcher.
// It then updates the coreMatcher with the new valueMatchers.
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

	freshFields.state.update(freshState)
	cm.updateable.Store(freshFields)

	return nil
}

// This function is not necessarily needed as `convertTrieNodeToValueMatcher` could be called directly but is left here for future use.
func convertPathTrieToValueMatcher(pt *pathTrie, vm *valueMatcher) error {
	err := convertTrieNodeToValueMatcher(pt.node, vm)
	if err != nil {
		return err
	}

	return nil
}

// This function is responsible for populating the valueMatcher with the appropriate smallTable.
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

// buildSmallTableFromTrie is responsible for building the smallTable from the trieNode.
// The whole point of this function is to create the smallTable in one shot so that we don't have to unpack and repack it later for performance reasons.
func buildSmallTableFromTrie(node *trieNode) (*smallTable, error) {
	size := len(node.children) + len(node.transition)
	states := make(map[byte]*faNext, size)
	epsilon := make([]*faState, 0)

	if node.isEnd || len(node.transition) > 0 {
		states[valueTerminator] = createNextTransition(node)
	}

	for ch, child := range node.children {
		if child.isWildcard && len(child.children) == 0 {
			continue
		}

		if child.isWildcard {
			wildcardState, nextStates := handleWildcardNode(child)
			epsilon = append(epsilon, wildcardState)
			for ch, next := range nextStates {
				if _, exists := states[ch]; !exists {
					states[ch] = next
				} else {
					states[ch].states = append(states[ch].states, next.states...)
				}
			}
			continue
		}

		nextState := getOrCreateState(child)
		if _, exists := states[ch]; !exists {
			states[ch] = &faNext{states: []*faState{nextState}}
		} else {
			states[ch].states = append(states[ch].states, nextState)
		}
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

	var table *smallTable
	if len(bytes) > 0 {
		table = makeSmallTable(nil, bytes, steps)
	} else {
		table = newSmallTable()
	}
	table.epsilon = epsilon

	return table, nil
}

// getOrCreateState is responsible for retrieving a cached faState from the globalStateCache or creating a new one if it doesn't exist.
func getOrCreateState(node *trieNode) *faState {
	key := getStateKey(node)
	if cachedState, exists := globalStateCache[key]; exists {
		return cachedState.states[0]
	}

	newState := &faState{
		table: newSmallTable(),
	}

	// Special handling for wildcard nodes
	for _, child := range node.children {
		if child.isWildcard && len(child.children) == 0 {
			transitionStates := createTransitionStates(child)
			for _, transitionState := range transitionStates {
				newState.fieldTransitions = append(newState.fieldTransitions, transitionState.fieldTransitions...)
			}
		}
	}
	childTable, _ := buildSmallTableFromTrie(node)
	newState.table = childTable

	globalStateCache[key] = &faNext{states: []*faState{newState}}
	return newState
}

// createNextTransition is responsible for creating a faNext structure for a given trieNode.
// It first checks the globalStateCache for an existing faNext structure. If one is found, it returns it.
// If no existing faNext structure is found, it creates a new one by calling createTransitionStates and then caches it in the globalStateCache.
func createNextTransition(node *trieNode) *faNext {
	key := getStateKey(node)
	if cachedState, exists := globalStateCache[key]; exists {
		return cachedState
	}

	states := createTransitionStates(node)

	if len(states) > 0 {
		result := &faNext{states: states}
		globalStateCache[key] = result
		return result
	}

	return nil
}

// createTransitionStates is responsible for creating the transition states for field and end transitions.
func createTransitionStates(node *trieNode) []*faState {
	var states []*faState

	// Handle end state if the node is an end node
	if node.isEnd {
		endState := &faState{
			table:            newSmallTable(),
			fieldTransitions: make([]*fieldMatcher, 0, len(node.memberOfPatterns)),
		}

		for x := range node.memberOfPatterns {
			fm := newFieldMatcher()
			fm.addMatch(x)
			endState.fieldTransitions = append(endState.fieldTransitions, fm)
		}

		states = append(states, endState)
	}

	// Handle transition state if the node has transitions
	if len(node.transition) > 0 {
		transitionState, _ := handleTransitions(node.transition)
		states = append(states, transitionState)
	}

	return states
}

// handleWildcardNode is responsible for handling the wildcard node.
// It creates a new faState and iterates over the children, creating the appropriate faNext structures.
func handleWildcardNode(node *trieNode) (*faState, map[byte]*faNext) {
	wildcardState := &faState{}

	states := make(map[byte]*faNext, len(node.children))

	if len(node.children) > 0 {
		for ch, child := range node.children {
			nextState := getOrCreateState(child)
			states[ch] = &faNext{states: []*faState{nextState}}
		}

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

		wildcardState.table = makeSmallTable(nil, bytes, steps)

		wildcardState.table.epsilon = []*faState{wildcardState}
	}

	return wildcardState, states
}

// handleTransitions is responsible for handling the transitions from the current path to the next path.
// It creates a new faState and iterates over the transitions, creating the appropriate valueMatchers.
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
		nextVM.fields().isNondeterministic = transitionNode.isNondeterministic
		nextVM.fields().hasNumbers = transitionNode.hasNumbers
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
