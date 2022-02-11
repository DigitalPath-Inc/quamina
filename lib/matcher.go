package quamina

// Matcher represents an automaton that allows matching sequences of name/value field pairs against
//  patterns, which are combinations of field names and lists of allowed valued field values.
// The field names are called "Paths" because they encode, in a jsonpath-ish style, the pathSegments from the
//  root of an incoming object to the leaf field.
// Since the order of fields is generally not significant in encoded data objects, the fields are sorted
//  by name before constructing the automaton, and so are the incoming field lists to be matched, allowing
//  the atutomaton to work.

import (
	"sort"
)

// Matcher uses a finite automaton to implement the MatchesForJSONEvent and MatchesForFields functions.
// startState is the start of the automaton
// namesUsed is a map of field names that are used in any of the patterns that this automaton encodes. Typically,
//  patterns only consider a subset of the fields in an incoming data object, and there is no reason to consider
//  fields that do not appear in patterns when using the automaton for matching
type Matcher struct {
	startState                *fieldMatchState
	namesUsed                 map[string]bool
	presumedExistFalseMatches *matchSet
	flattener                 Flattener
}

// X for anything, should eventually be a generic?
type X interface{}

func NewMatcher() *Matcher {
	m := Matcher{}
	m.startState = newFieldMatchState()
	m.namesUsed = make(map[string]bool)
	m.presumedExistFalseMatches = newMatchSet()
	m.flattener = NewFJ()
	return &m
}

// AddPattern - the patternBytes is a JSON object. The X is what the matcher returns to indicate that the
//  provided pattern has been matched. In many applications it might be a string which is the pattern's name.
func (m *Matcher) AddPattern(x X, patternJSON string) error {
	patternFields, patternNamesUsed, err := patternFromJSON([]byte(patternJSON))
	if err != nil {
		return err
	}
	for used := range patternNamesUsed {
		m.namesUsed[used] = true
	}
	sort.Slice(patternFields, func(i, j int) bool { return patternFields[i].path < patternFields[j].path })

	states := []*fieldMatchState{m.startState}
	for _, field := range patternFields {
		var nextStates []*fieldMatchState
		for _, state := range states {
			ns := state.addTransition(field)

			// special handling for exists:false, in which case there's only one next state
			if field.vals[0].vType == existsFalseType {
				ns[0].existsFalseFailures.addX(x)
				m.presumedExistFalseMatches.addX(x)
			}
			nextStates = append(nextStates, ns...)
		}
		states = nextStates
	}

	// "states" now holds the set of terminal states arrived at by matching each field in the pattern,
	//   so update the matches value to indicate this (skipping those that are only there to serve
	//   exists:false processing)
	for _, endState := range states {
		if !endState.existsFalseFailures.contains(x) {
			endState.matches = append(endState.matches, x)
		}
	}

	return err
}

// MatchesForJSONEvent calls the flattener to pull the fields out of the event and
//  hands over to MatchesForFields
func (m *Matcher) MatchesForJSONEvent(event []byte) ([]X, error) {
	m.flattener.Reset()
	fields, err := m.flattener.Flatten(event, m)
	if err != nil {
		return nil, err
	}
	matches := m.MatchesForFields(fields)
	return matches, nil
}

// MatchesForFields takes a list of Field structures and sorts them by pathname; the fields in a pattern to
//  matched are similarly sorted; thus running an automaton over them works
func (m *Matcher) MatchesForFields(fields []Field) []X {
	sort.Slice(fields, func(i, j int) bool { return string(fields[i].Path) < string(fields[j].Path) })
	return m.matchesForSortedFields(fields).matches()
}

// proposedTransition represents a suggestino that the name/value pair at fields[fieldIndex] might allow a transition
//  in the indicated state
type proposedTransition struct {
	state      *fieldMatchState
	fieldIndex int
}

// matchesForSortedFields runs the provided list of name/value pairs against the automaton and returns
//  a possibly-empty list of the patterns that match
func (m *Matcher) matchesForSortedFields(fields []Field) *matchSet {

	failedExistsFalseMatches := newMatchSet()

	// The idea is that we add potential field transitions to the proposals list; any time such a transition
	//  succeeds, i.e. matches a particular field and moves to a new state, we propose transitions from that
	//  state on all the following fields in the event
	// Start by giving each field a chance to match against the start state. Doing it by pre-allocating the
	//  proposals and filling in their values is observably faster than the more idiomatic append()
	proposals := make([]proposedTransition, len(fields))
	for i := range fields {
		proposals[i].fieldIndex = i
		proposals[i].state = m.startState
	}

	// as long as there are still potential transitions
	matches := newMatchSet()
	for len(proposals) > 0 {

		// go slices could usefully have a "pop" primitive
		lastEl := len(proposals) - 1
		proposal := proposals[lastEl]
		proposals = proposals[0:lastEl]

		// generate the possibly-empty list of transitions from state on the name/value pair
		nextStates := proposal.state.transitionOn(&fields[proposal.fieldIndex])

		// for each state in the set of transitions from the proposed state
		for _, nextState := range nextStates {

			// if arriving at this state means we've matched one or more patterns, record that fact
			for _, nextMatch := range nextState.matches {
				matches.addX(nextMatch)
			}

			// have we invalidated a presumed exists:false pattern?
			for existsMatch := range nextState.existsFalseFailures.set {
				failedExistsFalseMatches.addX(existsMatch)
			}

			// for each state we've transitioned to, give each subsequent field a chance to
			//  transition on it, assuming it's not in an object that's in a different element
			//  of the same array
			for nextIndex := proposal.fieldIndex + 1; nextIndex < len(fields); nextIndex++ {
				if noArrayTrailConflict(fields[proposal.fieldIndex].ArrayTrail, fields[nextIndex].ArrayTrail) {
					proposals = append(proposals, proposedTransition{fieldIndex: nextIndex, state: nextState})
				}
			}
		}
	}
	for presumedExistsFalseMatch := range m.presumedExistFalseMatches.set {
		if !failedExistsFalseMatches.contains(presumedExistsFalseMatch) {
			matches.addX(presumedExistsFalseMatch)
		}
	}
	return matches
}

// Arrays are invisible in the automaton.  That is to say, if an event has
//  { "a": [ 1, 2, 3 ] }
//  Then the fields will be a/1, a/2, and a/3
//  Same for  {"a": [[1, 2], 3]} or any other permutation
//  So if you have {"a": [ { "b": 1, "c": 2}, {"b": 3, "c": 4}] }
//  then a pattern like { "a": { "b": 1, "c": 4 } } would match.
// To prevent that from happening, each ArrayPos contains two numbers; the first identifies the array in
//  the event that this name/val occurred in, the second the position in the array. We don't allow
//  transitioning between field values that occur in different positions in the same array.
//  See the arrays_test unit test for more examples.
func noArrayTrailConflict(from []ArrayPos, to []ArrayPos) bool {
	for _, fromAPos := range from {
		for _, toAPos := range to {
			if fromAPos.Array == toAPos.Array && fromAPos.Pos != toAPos.Pos {
				return false
			}
		}
	}
	return true
}

func (m *Matcher) IsNameUsed(label []byte) bool {
	_, ok := m.namesUsed[string(label)]
	return ok
}
