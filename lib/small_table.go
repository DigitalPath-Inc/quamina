package quamina

// smallTable serves as a lookup table that encodes mappings between ranges of byte values and the smallTable you
//  transition to on any byte in the range.
//  The way it works is exposed in the step() function just below.  Logically, it's a slice of {byte, *smallTable}
//  but I imagine organizing it this way is a bit more memory-efficient.  Suppose we want to model a table where
//  byte values 3 and 4 (0-based) map to ss1 and byte 0x34 maps to ss2.  Then the smallTable would look like:
//  ceilings: 3,   5,    0x34, 0x35, Utf8ByteCeiling
//     steps: nil, &ss1, nil,  &ss2, nil
//  invariant: The last element of ceilings is always Utf8ByteCeiling
// The motivation is that we want to build a state machine on byte values to implement things like prefixes and
//  ranges of bytes.  This could be done simply with a byte array of size Utf8ByteCeiling for each state in the machine,
//  or a map[byte]smallStep, but both would be size-inefficient, particularly in the case where you're implementing
//  ranges.  Now, the step function is O(N) in the number of entries, but empirically, the number of entries is
//  small even in large machines, so skipping throgh the ceilings list is measurably about the same speed as a map
//  or array construct
type smallTable struct {
	ceilings   []byte
	states     []*smallTable
	transition *fieldMatchState
}

// Utf8ByteCeiling - the automaton runs on UTF-8 bytes, which map nicely to Go byte, which is uint8. The values
//  0xF5-0xFF can't appear in UTF-8 strings, so anything can safely be assumed to be less than this value
const Utf8ByteCeiling int = 0xf5

func newSmallTable() *smallTable {
	return &smallTable{
		ceilings: []byte{byte(Utf8ByteCeiling)},
		states:   []*smallTable{nil},
	}
}

func (t *smallTable) step(utf8Byte byte) *smallTable {
	for entry, ceiling := range t.ceilings {
		if utf8Byte < ceiling {
			return t.states[entry]
		}
	}
	panic("Malformed SmallTable")
}

// unpackedTable replicates the data in the smallTable ceilings and steps arrays.  It's quite hard to
//  update the list structure in a smallTable, but trivial in an unpackedTable and (looking forward)
//  will simplify atomic update. The idea is that to update a smallTable you unpack it, update, then
//  re-pack it.  Not gonna be the most efficient thing so at some future point…
// TODO: Figure out how to update a smallTable in place.
type unpackedTable [Utf8ByteCeiling]*smallTable

func unpack(t *smallTable) *unpackedTable {
	var u unpackedTable
	unpackedIndex := 0
	for packedIndex, c := range t.ceilings {
		ceiling := int(c)
		for unpackedIndex < ceiling {
			u[unpackedIndex] = t.states[packedIndex]
			unpackedIndex++
		}
	}
	return &u
}

func (t *smallTable) pack(u *unpackedTable) {
	t.ceilings = t.ceilings[:0]
	t.states = t.states[:0]
	last := u[0]
	for unpackedIndex, ss := range u {
		if ss != last {
			t.ceilings = append(t.ceilings, byte(unpackedIndex))
			t.states = append(t.states, last)
		}
		last = ss
	}
	t.ceilings = append(t.ceilings, byte(Utf8ByteCeiling))
	t.states = append(t.states, last)
}

func (t *smallTable) addRange(utf8Bytes []byte, state *smallTable) {
	// TODO update fuzz test to include this
	unpacked := unpack(t)
	for _, utf8Byte := range utf8Bytes {
		unpacked[utf8Byte] = state
	}
	t.pack(unpacked)
}
