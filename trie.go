package quamina

import (
	"fmt"
	"hash/fnv"
	"sort"
	"strconv"
	"strings"
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
	hash             uint64
}

func (n *trieNode) generateHash() uint64 {
	h := fnv.New64a()

	// Hash basic properties
	h.Write([]byte{btoi(n.isEnd), btoi(n.isWildcard)})

	// Hash memberOfPatterns
	if len(n.memberOfPatterns) > 0 {
		patternKeys := make([]string, 0, len(n.memberOfPatterns))
		for k := range n.memberOfPatterns {
			patternKeys = append(patternKeys, k.(string))
		}
		if len(patternKeys) > 1 {
			sort.Slice(patternKeys, func(i, j int) bool { return patternKeys[i] < patternKeys[j] })
		}
		for _, k := range patternKeys {
			h.Write([]byte(k))
		}
	}

	// Hash children (bottom-up)
	if len(n.children) > 0 {
		childKeys := make([]byte, 0, len(n.children))
		for k := range n.children {
			childKeys = append(childKeys, k)
		}
		if len(childKeys) > 1 {
			sort.Slice(childKeys, func(i, j int) bool { return childKeys[i] < childKeys[j] })
		}
		for _, k := range childKeys {
			h.Write([]byte{k})
			childHash := n.children[k].generateHash() // Recursive call
			h.Write([]byte(strconv.FormatUint(childHash, 10)))
		}
	}

	// Hash transitions (bottom-up)
	if len(n.transition) > 0 {
		transitionKeys := make([]string, 0, len(n.transition))
		for k := range n.transition {
			transitionKeys = append(transitionKeys, k)
		}
		if len(transitionKeys) > 1 {
			sort.Slice(transitionKeys, func(i, j int) bool { return transitionKeys[i] < transitionKeys[j] })
		}
		for _, k := range transitionKeys {
			h.Write([]byte(k))
			transitionHash := n.transition[k].node.generateHash() // Recursive call
			h.Write([]byte(strconv.FormatUint(transitionHash, 10)))
		}
	}

	n.hash = h.Sum64() // Store the computed hash
	return n.hash
}

type stateCache struct {
	cache map[uint64]*faNext
}

func (sc *stateCache) get(node *trieNode) (*faNext, bool) {
	hash := node.hash
	state, exists := sc.cache[hash]
	return state, exists
}

func (sc *stateCache) set(node *trieNode, state *faNext) {
	hash := node.hash
	sc.cache[hash] = state
}

func newStateCache() *stateCache {
	return &stateCache{
		cache: make(map[uint64]*faNext),
	}
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
	fields := make(map[string]struct{})
	root, err := trieFromPatterns(patterns, &fields)
	if err != nil {
		return nil, err
	}

	cm := newCoreMatcher()
	err = convertTrieToCoreMatcher(cm, root)
	if err != nil {
		return nil, err
	}

	segmentsTree := newSegmentsIndex()
	for field := range fields {
		segmentsTree.add(field)
	}
	cm.fields().segmentsTree = segmentsTree

	return cm, nil
}

// Modify the trieFromPatterns function to use the cache
func trieFromPatterns(patterns map[X]string, allFields *map[string]struct{}) (map[string]*pathTrie, error) {
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
		buildTrieTime += time.Since(buildTrieStart)
		if err != nil {
			return nil, err
		}

		for _, field := range fields {
			(*allFields)[field.path] = struct{}{}
		}
	}

	// Generate hash for each trie
	for _, trie := range root {
		trie.node.generateHash()
	}

	return root, nil
}

// Modify the buildTrie function to use the cache
func buildTrie(root map[string]*pathTrie, fields []*patternField, x X) error {
	if len(fields) == 0 {
		return nil
	}

	currentField := fields[0]
	remainingFields := fields[1:]

	// Create a new trie for the current field if it doesn't exist
	if _, exists := root[currentField.path]; !exists {
		root[currentField.path] = newPathTrie(currentField.path)
	}

	trie := root[currentField.path]

	for _, val := range currentField.vals {
		var err error
		switch val.vType {
		case stringType, literalType, numberType:
			err = insertStringValue(trie, val.val, remainingFields, x)
		case prefixType:
			err = insertPrefixValue(trie, val.val, remainingFields, x)
		case monocaseType:
			err = insertMonocaseValue(trie, val.val, remainingFields, x)
		default:
			return fmt.Errorf("unknown value type: %v", val.vType)
		}
		if err != nil {
			return err
		}
	}

	// If there are remaining fields, create a transition to the next field
	if len(remainingFields) > 0 {
		nextField := remainingFields[0]
		if trie.node.transition == nil {
			trie.node.transition = make(map[string]*pathTrie)
		}
		if _, exists := trie.node.transition[nextField.path]; !exists {
			trie.node.transition[nextField.path] = newPathTrie(nextField.path)
		}
		// Change: Only recurse on the transition, not the root
		err := buildTrie(trie.node.transition, remainingFields, x)
		if err != nil {
			return err
		}
	}

	return nil
}

// Modify the insert functions to use the cache
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
		}
		// Change: Use the transition map instead of creating a new root map
		return buildTrie(node.transition, remainingFields, x)
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
		}
		return buildTrie(map[string]*pathTrie{remainingFields[0].path: node.transition[remainingFields[0].path]}, remainingFields, x)
	}

	return nil
}

// insertMonocaseValue uses the same logic as insertStringValue but handles case folding. It basically generates two paths for each rune in the value
// and adds them to the trie. We first generate all the permutations of the value, then add each as a trie
func insertMonocaseValue(trie *pathTrie, value string, remainingFields []*patternField, x X) error {
	permutations := getAltPermutations(value)
	for _, perm := range permutations {
		err := insertStringValue(trie, perm, remainingFields, x)
		if err != nil {
			return err
		}
	}
	return nil
}

func getAltPermutations(value string) []string {
	permutations := []string{value}
	for i, r := range value {
		if altRune, ok := caseFoldingPairs[r]; ok {
			newPermutations := make([]string, len(permutations))
			for j, perm := range permutations {
				newPerm := perm[:i] + string(altRune) + perm[i+1:]
				newPermutations[j] = newPerm
			}
			permutations = append(permutations, newPermutations...)
		}
	}
	return permutations
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

	sc := newStateCache()

	for path, trie := range root {
		vm := newValueMatcher()
		err := convertPathTrieToValueMatcher(trie, vm, sc)
		if err != nil {
			return err
		}
		freshState.transitions[path] = vm
	}

	freshFields.state.update(freshState)
	cm.updateable.Store(freshFields)

	return nil
}

func convertPathTrieToValueMatcher(pt *pathTrie, vm *valueMatcher, cache *stateCache) error {
	err := convertTrieNodeToValueMatcher(pt.node, vm, cache)
	if err != nil {
		return err
	}

	return nil
}

func convertTrieNodeToValueMatcher(node *trieNode, vm *valueMatcher, cache *stateCache) error {
	fields := vm.getFieldsForUpdate()

	table, err := buildSmallTableFromTrie(node, cache)
	if err != nil {
		return err
	}
	fields.startTable = table

	vm.update(fields)
	return nil
}

func buildSmallTableFromTrie(node *trieNode, cache *stateCache) (*smallTable, error) {
	size := len(node.children) + len(node.transition)
	states := make(map[byte]*faNext, size)

	if node.isEnd || node.isWildcard {
		createEndState(node, &states, cache)
	}

	if len(node.transition) > 0 {
		createTransitionState(node, &states, cache)
	}

	for ch, child := range node.children {
		nextState := getOrCreateState(child, cache)
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

func createEndState(node *trieNode, states *map[byte]*faNext, cache *stateCache) {
	if cachedState, exists := cache.get(node); exists {
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

	nextState := &faNext{states: []*faState{endState}}
	cache.set(node, nextState)
	(*states)[valueTerminator] = nextState
}

func createTransitionState(node *trieNode, states *map[byte]*faNext, cache *stateCache) {
	if cachedState, exists := cache.get(node); exists {
		(*states)[valueTerminator] = cachedState
		return
	}

	transitionState, _ := handleTransitions(node.transition, cache)
	nextState := &faNext{states: []*faState{transitionState}}
	cache.set(node, nextState)
	(*states)[valueTerminator] = nextState
}

func createWildcardState(node *trieNode, nextState *faState, cache *stateCache) error {
	for x := range node.memberOfPatterns {
		fm := newFieldMatcher()
		fm.addMatch(x)
		nextState.fieldTransitions = append(nextState.fieldTransitions, fm)
	}

	if len(node.transition) > 0 {
		transitionState, err := handleTransitions(node.transition, cache)
		if err != nil {
			return err
		}
		nextState.fieldTransitions = append(nextState.fieldTransitions, transitionState.fieldTransitions...)
	}

	return nil
}

func getStateKey(node *trieNode, depth int) string {
	var key strings.Builder
	key.Grow(64) // Pre-allocate some space to reduce allocations

	// Add basic properties
	key.WriteString("e:")
	key.WriteByte('0' + btoi(node.isEnd))
	key.WriteString("|w:")
	key.WriteByte('0' + btoi(node.isWildcard))
	key.WriteByte('|')

	// Add patterns
	if len(node.memberOfPatterns) > 0 {
		key.WriteString("p:")
		patterns := make([]string, 0, len(node.memberOfPatterns))
		for x := range node.memberOfPatterns {
			patterns = append(patterns, x.(string))
		}
		sort.Strings(patterns)
		key.WriteString(strings.Join(patterns, ","))
		key.WriteByte('|')
	}

	// Add children (with limited depth)
	if len(node.children) > 0 && depth > 0 {
		key.WriteString("c:")
		childKeys := make([]string, 0, len(node.children))
		for ch, child := range node.children {
			childKey := fmt.Sprintf("%d:%s", ch, getStateKey(child, depth-1))
			childKeys = append(childKeys, childKey)
		}
		sort.Strings(childKeys)
		key.WriteString(strings.Join(childKeys, ","))
		key.WriteByte('|')
	}

	// Add transitions (with limited depth)
	if len(node.transition) > 0 && depth > 0 {
		key.WriteString("t:")
		transitions := make([]string, 0, len(node.transition))
		for t, pathTrie := range node.transition {
			transitionKey := fmt.Sprintf("%s:%s", t, getStateKey(pathTrie.node, depth-1))
			transitions = append(transitions, transitionKey)
		}
		sort.Strings(transitions)
		key.WriteString(strings.Join(transitions, ","))
	}

	return key.String()
}

// Helper function to convert bool to int
func btoi(b bool) byte {
	if b {
		return 1
	}
	return 0
}

func handleTransitions(transitions map[string]*pathTrie, cache *stateCache) (*faState, error) {
	transitionVMs := make(map[string]*valueMatcher)
	nextState := &faState{
		table:            newSmallTable(),
		fieldTransitions: make([]*fieldMatcher, 0),
	}

	for nextFieldPath, transitionNode := range transitions {
		nextVM := newValueMatcher()
		err := convertPathTrieToValueMatcher(transitionNode, nextVM, cache)
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

func getOrCreateState(node *trieNode, cache *stateCache) *faState {
	if cachedState, exists := cache.get(node); exists {
		return cachedState.states[0]
	}

	newState := &faState{
		table: newSmallTable(),
	}

	if node.isWildcard {
		createWildcardState(node, newState, cache)
	} else {
		childTable, _ := buildSmallTableFromTrie(node, cache)
		newState.table = childTable
	}

	cache.set(node, &faNext{states: []*faState{newState}})
	return newState
}
