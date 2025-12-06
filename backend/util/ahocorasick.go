// Package ahocorasick implements the Aho-Corasick string matching algorithm for
// efficiently finding all instances of multiple patterns in a text.
package util

import (
	"bytes"
	"fmt"
	"sort"
)

const (
	// leaf represents a leaf on the trie
	// This must be <255 since the offsets used are in [0,255]
	// This should only appear in the Base array since the Check array uses
	// negative values to represent free states.
	leaf = -1867
)

// Matcher is the pattern matching state machine.
// Optimized to use int32 to save memory (50% reduction on 64-bit systems).
type Matcher struct {
	base  []int32 // base array in the double array trie
	check []int32 // check array in the double array trie
	fail  []int32 // fail function

	// Flattened output table to reduce GC pressure and allocations
	// outputIndex[state] points to the start of matches in outputs
	// outputLens[state] is the number of matches for the state
	// outputs contains the lengths of matched words
	outputIndex []int32
	outputLens  []int32
	outputs     []int32
}

func (m *Matcher) String() string {
	return fmt.Sprintf(`
Base:   %v
Check:  %v
Fail:   %v
OutputIndex: %v
OutputLens: %v
Outputs: %v
`, m.base, m.check, m.fail, m.outputIndex, m.outputLens, m.outputs)
}

type byteSliceSlice [][]byte

func (bss byteSliceSlice) Len() int           { return len(bss) }
func (bss byteSliceSlice) Less(i, j int) bool { return bytes.Compare(bss[i], bss[j]) < 1 }
func (bss byteSliceSlice) Swap(i, j int)      { bss[i], bss[j] = bss[j], bss[i] }

func compile(words [][]byte) *Matcher {
	m := new(Matcher)
	initialSize := 2048
	m.base = make([]int32, initialSize)[:1]
	m.check = make([]int32, initialSize)[:1]
	m.fail = make([]int32, initialSize)[:1]

	// Temporary output storage during construction
	tempOutput := make([][]int, initialSize)[:1]

	sort.Sort(byteSliceSlice(words))

	// Represents a node in the implicit trie of words
	type trienode struct {
		state int
		depth int
		start int
		end   int
	}
	queue := make([]trienode, initialSize)[:1]
	queue[0] = trienode{0, 0, 0, len(words)}

	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]

		if node.end <= node.start {
			m.base[node.state] = leaf
			continue
		}

		var edges []byte
		for i := node.start; i < node.end; i++ {
			if len(edges) == 0 || edges[len(edges)-1] != words[i][node.depth] {
				edges = append(edges, words[i][node.depth])
			}
		}

		// Calculate a suitable Base value where each edge will fit into the
		// double array trie
		base := m.findBase(edges)
		m.base[node.state] = int32(base)

		i := node.start
		for _, edge := range edges {
			offset := int(edge)
			newState := base + offset

			// Grow tempOutput if needed to accommodate newState
			if newState >= len(tempOutput) {
				extra := newState - len(tempOutput) + 1
				// Grow by at least chunk to reduce allocations
				if extra < 256 {
					extra = 256
				}
				tempOutput = append(tempOutput, make([][]int, extra)...)
			}

			m.occupyState(newState, node.state)

			// level 0 and level 1 should fail to state 0
			if node.depth > 0 {
				m.setFailState(newState, node.state, offset)
			}

			// Union fail output
			failState := int(m.fail[newState])
			if len(tempOutput[failState]) > 0 {
				tempOutput[newState] = append([]int{}, tempOutput[failState]...)
			}

			// Add the child nodes to the queue to continue down the BFS
			newnode := trienode{newState, node.depth + 1, i, i}
			for {
				if newnode.depth >= len(words[i]) {
					tempOutput[newState] = append(tempOutput[newState], len(words[i]))
					newnode.start++
				}
				newnode.end++

				i++
				if i >= node.end || words[i][node.depth] != edge {
					break
				}
			}
			queue = append(queue, newnode)
		}
	}

	// Flatten output
	m.flattenOutput(tempOutput)

	return m
}

func (m *Matcher) flattenOutput(tempOutput [][]int) {
	size := len(tempOutput)
	m.outputIndex = make([]int32, size)
	m.outputLens = make([]int32, size)

	var totalLen int
	for _, lens := range tempOutput {
		totalLen += len(lens)
	}

	m.outputs = make([]int32, 0, totalLen)

	currentIdx := int32(0)
	for i, lens := range tempOutput {
		if len(lens) == 0 {
			m.outputIndex[i] = 0
			m.outputLens[i] = 0
			continue
		}
		m.outputIndex[i] = currentIdx
		m.outputLens[i] = int32(len(lens))
		for _, l := range lens {
			m.outputs = append(m.outputs, int32(l))
		}
		currentIdx += int32(len(lens))
	}
}

// CompileByteSlices compiles a Matcher from a slice of byte slices. This Matcher can be
// used to find occurrences of each pattern in a text.
func CompileByteSlices(words [][]byte) *Matcher {
	return compile(words)
}

// CompileStrings compiles a Matcher from a slice of strings. This Matcher can
// be used to find occurrences of each pattern in a text.
func CompileStrings(words []string) *Matcher {
	var wordByteSlices [][]byte
	for _, word := range words {
		wordByteSlices = append(wordByteSlices, []byte(word))
	}
	return compile(wordByteSlices)
}

// occupyState will correctly occupy state so it maintains the
// index=check[base[index]+offset] identity. It will also update the
// bidirectional link of free states correctly.
func (m *Matcher) occupyState(state, parentState int) {
	firstFreeState := m.firstFreeState()
	lastFreeState := m.lastFreeState()

	s := int32(state)
	ps := int32(parentState)

	if firstFreeState == lastFreeState {
		m.check[0] = 0
	} else {
		switch state {
		case firstFreeState:
			next := -1 * int(m.check[state])
			m.check[0] = int32(-1 * next)
			m.base[next] = m.base[state]
		case lastFreeState:
			prev := -1 * int(m.base[state])
			m.base[firstFreeState] = int32(-1 * prev)
			m.check[prev] = -1
		default:
			next := -1 * int(m.check[state])
			prev := -1 * int(m.base[state])
			m.check[prev] = int32(-1 * next)
			m.base[next] = int32(-1 * prev)
		}
	}
	m.check[s] = ps
	m.base[s] = leaf
}

// setFailState sets the output of the fail function for input state.
func (m *Matcher) setFailState(state, parentState, offset int) {
	failState := int(m.fail[parentState])
	for {
		if m.hasEdge(failState, offset) {
			m.fail[state] = m.base[failState] + int32(offset)
			break
		}
		if failState == 0 {
			break
		}
		failState = int(m.fail[failState])
	}
}

// findBase finds a base value which has free states in the positions that
// correspond to each edge transition in edges.
func (m *Matcher) findBase(edges []byte) int {
	if len(edges) == 0 {
		return leaf
	}

	min := int(edges[0])
	max := int(edges[len(edges)-1])
	width := max - min
	freeState := m.firstFreeState()
	for freeState != -1 {
		valid := true
		for _, e := range edges[1:] {
			state := freeState + int(e) - min
			if state >= len(m.check) {
				break
			} else if m.check[state] >= 0 {
				valid = false
				break
			}
		}

		if valid {
			if freeState+width >= len(m.check) {
				m.increaseSize(width - len(m.check) + freeState + 1)
			}
			return freeState - min
		}

		freeState = m.nextFreeState(freeState)
	}
	freeState = len(m.check)
	m.increaseSize(width + 1)
	return freeState - min
}

// increaseSize increases the size of arrays to ensure they remain the same size.
func (m *Matcher) increaseSize(dsize int) {
	if dsize == 0 {
		return
	}

	m.base = append(m.base, make([]int32, dsize)...)
	m.check = append(m.check, make([]int32, dsize)...)
	m.fail = append(m.fail, make([]int32, dsize)...)

	// Flattened output arrays are built at the end, but we need to match index space if accessed during compile?
	// During compile we use tempOutput, which needs to grow.
	// But `increaseSize` is called during `findBase`, which is called inside compile loop.
	// We need to grow tempOutput here too if we want to access it by state index.
	// However, `increaseSize` is mainly for base/check/fail. `compile` function uses `queue` and manages states.
	// The `tempOutput` slice in `compile` must also be grown.
	// But `increaseSize` is a method on `Matcher`, it doesn't have access to `tempOutput` local variable in `compile`.
	// We should probably include tempOutput in Matcher or handle resizing differently.
	// Actually, `compile` initializes `m` with capacity 2048. `increaseSize` appends.
	// We need to sync `tempOutput` growth.
	// The best way is to pre-allocate or handle it.
	// Since we can't easily access `tempOutput` from here, we will handle `tempOutput` in `compile`?
	// No, `increaseSize` is called deep in `findBase`.
	// Let's modify `increaseSize` to NOT handle output, but we need to ensure `tempOutput` is large enough in `compile`.
	// Wait, `tempOutput` is indexed by `state`. `increaseSize` increases `len(m.check)`.
	// The new states are added to `base` and `check`.
	// `tempOutput` needs to be grown to match `len(m.check)`.
	// Since I cannot change signature of `increaseSize` easily without updating all calls (which is fine),
	// or I can make `tempOutput` a field of `Matcher` temporarily?
	// No, `Matcher` struct should correspond to compiled state.
	// Let's change `compile` to pass a callback or specific handling? No.
	// Let's just create `tempOutput` with a large capacity or resize it when assigning?
	// Only `compile` writes to `tempOutput`.
	// `tempOutput` is accessed by `state`.
	// If `state` grows beyond `len(tempOutput)`, we panic.
	// `state` comes from `newState = base + offset`.
	// `base` is determined by `findBase` which calls `increaseSize`.
	// So `len(m.base)` determines max state index.
	// We CANNOT access `tempOutput` inside `increaseSize`.

	// Refactoring: We will just resize `tempOutput` lazily in `compile` loop whenever we encounter a state >= len.
	// Or even simpler: Use `make([][]int, len(m.base))` after compilation? No, we need it during BFS.
	// We can check `if newState >= len(tempOutput)` in `compile` loop and grow it.

	lastFreeState := m.lastFreeState()
	firstFreeState := m.firstFreeState()

	startLen := len(m.check) - dsize
	for i := startLen; i < len(m.check); i++ {
		if lastFreeState == -1 {
			m.check[0] = int32(-1 * i)
			m.base[i] = int32(-1 * i)
			m.check[i] = -1
			firstFreeState = i
			lastFreeState = i
		} else {
			m.base[i] = int32(-1 * lastFreeState)
			m.check[i] = -1
			m.base[firstFreeState] = int32(-1 * i)
			m.check[lastFreeState] = int32(-1 * i)
			lastFreeState = i
		}
	}
}

// nextFreeState uses the nature of the bidirectional link to determine the
// closest free state at a larger index.
func (m *Matcher) nextFreeState(curFreeState int) int {
	nextState := -1 * int(m.check[curFreeState])

	// state 1 can never be a free state.
	if nextState == 1 {
		return -1
	}

	return nextState
}

// firstFreeState uses the first value in the check array which points to the
// first free state.
func (m *Matcher) firstFreeState() int {
	state := int(m.check[0])
	if state != 0 {
		return -1 * state
	}
	return -1
}

// lastFreeState uses the base value of the first free state which points the
// last free state.
func (m *Matcher) lastFreeState() int {
	firstFree := m.firstFreeState()
	if firstFree != -1 {
		return -1 * int(m.base[firstFree])
	}
	return -1
}

// hasEdge determines if the fromState has a transition for offset.
func (m *Matcher) hasEdge(fromState, offset int) bool {
	toState := int(m.base[fromState]) + offset
	return toState > 0 && toState < len(m.check) && int(m.check[toState]) == fromState
}

// Match represents a matched pattern in the text
type Match struct {
	Word  []byte // the matched pattern
	Index int    // the start index of the match
}

func (m *Match) String() string {
	return fmt.Sprintf(`{ "%s" %d }`, m.Word, m.Index)
}

func (m *Matcher) findAll(text []byte) []*Match {
	var matches []*Match
	m.Iterate(text, func(word []byte, index int) bool {
		// iterate returns match, index (start).
		// We copy the slice because the callback slice might be reused if we optimized it (here it is slice of text).
		// Actually, standard AC returns reference to patterns.
		// My Iterate implementation below returns slice of text.
		matches = append(matches, &Match{Word: word, Index: index})
		return true
	})
	return matches
}

// Iterate scans the text and calls the callback for each match found.
// The callback receives the matched word (as a slice of the text) and the start index.
// If the callback returns false, iteration stops.
// This method avoids allocating a slice of matches.
func (m *Matcher) Iterate(text []byte, fn func(word []byte, start int) bool) {
	state := 0
	for i, b := range text {
		offset := int(b)
		for state != 0 && !m.hasEdge(state, offset) {
			state = int(m.fail[state])
		}

		if m.hasEdge(state, offset) {
			state = int(m.base[state]) + offset
		}

		if m.outputLens[state] > 0 {
			idx := m.outputIndex[state]
			count := m.outputLens[state]
			for j := int32(0); j < count; j++ {
				wordLen := int(m.outputs[idx+j])
				if !fn(text[i-wordLen+1:i+1], i-wordLen+1) {
					return
				}
			}
		}
	}
}

// IterateString scans the string text and calls the callback for each match found.
func (m *Matcher) IterateString(text string, fn func(word string, start int) bool) {
	// To avoid unsafe conversion or allocation if possible, we can just cast to []byte
	// but that might allocate.
	// Since we need to slice it, we can just work with the string.
	// We can implement a parallel IterateString logic or convert.
	// For now, convert to []byte to reuse logic but this allocates for the slice header?
	// Actually text matches are substrings.
	// Let's implement native string iteration to avoid []byte conversion if user passed string.

	state := 0
	for i := 0; i < len(text); i++ {
		offset := int(text[i])
		for state != 0 && !m.hasEdge(state, offset) {
			state = int(m.fail[state])
		}

		if m.hasEdge(state, offset) {
			state = int(m.base[state]) + offset
		}

		if m.outputLens[state] > 0 {
			idx := m.outputIndex[state]
			count := m.outputLens[state]
			for j := int32(0); j < count; j++ {
				wordLen := int(m.outputs[idx+j])
				if !fn(text[i-wordLen+1:i+1], i-wordLen+1) {
					return
				}
			}
		}
	}
}

// FindAllByteSlice finds all instances of the patterns in the text.
func (m *Matcher) FindAllByteSlice(text []byte) (matches []*Match) {
	return m.findAll(text)
}

// FindAllString finds all instances of the patterns in the text.
func (m *Matcher) FindAllString(text string) []*Match {
	var matches []*Match
	m.IterateString(text, func(word string, index int) bool {
		matches = append(matches, &Match{Word: []byte(word), Index: index})
		return true
	})
	return matches
}
