package quamina

import (
	"bytes"
	"fmt"
	"strings"
	"unsafe"
)

type mermaidTrieVisualizer struct {
	buf          *bytes.Buffer
	visitedNodes map[uintptr]string
	visitedLinks map[string]bool
	endNodes     map[uintptr]bool
}

func newMermaidTrieVisualizer() *mermaidTrieVisualizer {
	return &mermaidTrieVisualizer{
		buf:          &bytes.Buffer{},
		visitedNodes: make(map[uintptr]string),
		visitedLinks: make(map[string]bool),
		endNodes:     make(map[uintptr]bool),
	}
}

func (mv *mermaidTrieVisualizer) reset() {
	mv.buf.Reset()
	mv.visitedNodes = make(map[uintptr]string)
	mv.visitedLinks = make(map[string]bool)
	mv.endNodes = make(map[uintptr]bool)
}

func (mv *mermaidTrieVisualizer) visualize(tries map[string]*pathTrie) string {
	mv.buf.WriteString("graph TD\n")
	mv.visualizeTries(tries)
	return mv.buf.String()
}

func (mv *mermaidTrieVisualizer) visualizeTries(tries map[string]*pathTrie) string {
	rootId := "root"
	mv.buf.WriteString(fmt.Sprintf("    %s[Root]\n", rootId))

	for field, trie := range tries {
		trieId := fmt.Sprintf("%p", trie)
		linkKey := fmt.Sprintf("%s-->%s", rootId, trieId)
		if !mv.visitedLinks[linkKey] {
			mv.buf.WriteString(fmt.Sprintf("    %s --> %s[%s]\n", rootId, trieId, field))
			mv.visitedLinks[linkKey] = true
		}
		mv.visualizePathTrieRecursive(trie, trieId, 0)
	}

	return mv.buf.String()
}

func (mv *mermaidTrieVisualizer) visualizePathTrieRecursive(trie *pathTrie, parentId string, depth int) {
	currentPtr := uintptr(unsafe.Pointer(trie))
	currentId, exists := mv.visitedNodes[currentPtr]
	if !exists {
		currentId = fmt.Sprintf("%p", trie)
		mv.visitedNodes[currentPtr] = currentId
		mv.buf.WriteString(fmt.Sprintf("    %s[%s<br />%p<br />%d]\n", currentId, trie.path, trie, trie.node.hash))
	}

	mv.visualizeTrieNode(trie.node, currentId, depth+1)

	for path, nextTrie := range trie.node.transition {
		nextPtr := uintptr(unsafe.Pointer(nextTrie))
		nextId, exists := mv.visitedNodes[nextPtr]
		if !exists {
			nextId = fmt.Sprintf("%p", nextTrie)
			mv.visitedNodes[nextPtr] = nextId
			mv.buf.WriteString(fmt.Sprintf("    %s[%s<br />%p]\n", nextId, path, nextTrie))
		}

		linkKey := fmt.Sprintf("%s-->%s", currentId, nextId)
		if !mv.visitedLinks[linkKey] {
			mv.buf.WriteString(fmt.Sprintf("    %s --> %s\n", currentId, nextId))
			mv.visitedLinks[linkKey] = true
		}

		mv.visualizePathTrieRecursive(nextTrie, currentId, depth+1)
	}
}

func (mv *mermaidTrieVisualizer) visualizeTrieNode(node *trieNode, parentId string, depth int) {
	currentPtr := uintptr(unsafe.Pointer(node))
	currentId, exists := mv.visitedNodes[currentPtr]
	if !exists {
		currentId = fmt.Sprintf("%p", node)
		mv.visitedNodes[currentPtr] = currentId
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
		if _, exists := mv.endNodes[currentPtr]; !exists {
			mv.buf.WriteString(fmt.Sprintf("    %s((%s<br />%p<br />%s))\n", endId, endLabel, node, patternsStr))
			mv.endNodes[currentPtr] = true
		}

		endLinkKey := fmt.Sprintf("%s-->%s", parentId, endId)
		if !mv.visitedLinks[endLinkKey] {
			mv.buf.WriteString(fmt.Sprintf("    %s --> %s\n", parentId, endId))
			mv.visitedLinks[endLinkKey] = true
		}
	}

	for ch, child := range node.children {
		childPtr := uintptr(unsafe.Pointer(child))
		childId, exists := mv.visitedNodes[childPtr]
		if !exists {
			childId = fmt.Sprintf("%p", child)
			mv.visitedNodes[childPtr] = childId
			if ch == '"' {
				mv.buf.WriteString(fmt.Sprintf("    %s[QUOTE<br />%p]\n", childId, child))
			} else {
				mv.buf.WriteString(fmt.Sprintf("    %s[%c<br />%p<br />%d]\n", childId, ch, child, child.hash))
			}

		}

		linkKey := fmt.Sprintf("%s-->%s", parentId, childId)
		if !mv.visitedLinks[linkKey] {
			mv.buf.WriteString(fmt.Sprintf("    %s --> %s\n", parentId, childId))
			mv.visitedLinks[linkKey] = true
		}

		mv.visualizeTrieNode(child, childId, depth+1)
	}

	for path, nextTrie := range node.transition {
		nextPtr := uintptr(unsafe.Pointer(nextTrie))
		nextId, exists := mv.visitedNodes[nextPtr]
		if !exists {
			nextId = fmt.Sprintf("%p", nextTrie)
			mv.visitedNodes[nextPtr] = nextId
			mv.buf.WriteString(fmt.Sprintf("    %s[%s<br />%p]\n", nextId, path, nextTrie))
		}

		linkKey := fmt.Sprintf("%s-->%s", parentId, nextId)
		if !mv.visitedLinks[linkKey] {
			mv.buf.WriteString(fmt.Sprintf("    %s --> %s\n", parentId, nextId))
			mv.visitedLinks[linkKey] = true
		}

		mv.visualizePathTrieRecursive(nextTrie, currentId, depth+1)
	}

}
