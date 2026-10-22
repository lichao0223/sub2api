package tokenestimate

import "container/heap"

type tokenPair struct{ left, right int }
type tokenMerge struct{ rank, id int }
type mergeCandidate struct {
	left, right int
	tokenMerge
}
type mergeQueue []mergeCandidate

func (q mergeQueue) Len() int { return len(q) }
func (q mergeQueue) Less(i, j int) bool {
	if q[i].rank != q[j].rank {
		return q[i].rank < q[j].rank
	}
	return q[i].left < q[j].left
}
func (q mergeQueue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }
func (q *mergeQueue) Push(x any) {
	candidate, ok := x.(mergeCandidate)
	if !ok {
		panic("tokenestimate: invalid merge candidate")
	}
	*q = append(*q, candidate)
}
func (q *mergeQueue) Pop() any {
	last := len(*q) - 1
	x := (*q)[last]
	*q = (*q)[:last]
	return x
}

func (t *deepSeekTokenizer) countPiece(text string) int {
	type symbol struct{ id, prev, next int }
	symbols := make([]symbol, len(text))
	for i := range symbols {
		symbols[i] = symbol{id: t.byteID[text[i]], prev: i - 1, next: i + 1}
	}
	var queue mergeQueue
	candidate := func(left int) (mergeCandidate, bool) {
		if left < 0 || left >= len(symbols) || symbols[left].id < 0 {
			return mergeCandidate{}, false
		}
		right := symbols[left].next
		if right >= len(symbols) {
			return mergeCandidate{}, false
		}
		merge, ok := t.merges[tokenPair{symbols[left].id, symbols[right].id}]
		return mergeCandidate{left: left, right: right, tokenMerge: merge}, ok
	}
	for i := range symbols {
		if c, ok := candidate(i); ok {
			queue = append(queue, c)
		}
	}
	heap.Init(&queue)
	count := len(symbols)
	for queue.Len() > 0 {
		c := queue[0]
		heap.Pop(&queue)
		// Neighboring merges invalidate queued pairs; equal ranks merge leftmost first.
		if current, ok := candidate(c.left); !ok || current != c {
			continue
		}
		symbols[c.left].id = c.id
		symbols[c.left].next = symbols[c.right].next
		symbols[c.right].id = -1
		if next := symbols[c.left].next; next < len(symbols) {
			symbols[next].prev = c.left
		}
		count--
		for _, left := range [2]int{symbols[c.left].prev, c.left} {
			if next, ok := candidate(left); ok {
				heap.Push(&queue, next)
			}
		}
	}
	return count
}
