package tokenestimate

import "slices"

type tokenPair struct{ left, right int }
type tokenMerge struct{ rank, id int }
type mergeCandidate struct {
	left, right int
	tokenMerge
}
type mergeQueue []mergeCandidate

func (q mergeQueue) less(i, j int) bool {
	if q[i].rank != q[j].rank {
		return q[i].rank < q[j].rank
	}
	return q[i].left < q[j].left
}
func (q mergeQueue) down(i int) {
	for {
		child := 2*i + 1
		if child >= len(q) {
			return
		}
		if right := child + 1; right < len(q) && q.less(right, child) {
			child = right
		}
		if !q.less(child, i) {
			return
		}
		q[i], q[child] = q[child], q[i]
		i = child
	}
}

func (q *mergeQueue) push(candidate mergeCandidate) {
	*q = append(*q, candidate)
	for i := len(*q) - 1; i > 0; {
		parent := (i - 1) / 2
		if !q.less(i, parent) {
			break
		}
		(*q)[i], (*q)[parent] = (*q)[parent], (*q)[i]
		i = parent
	}
}

func (q *mergeQueue) pop() mergeCandidate {
	first := (*q)[0]
	last := len(*q) - 1
	(*q)[0] = (*q)[last]
	*q = (*q)[:last]
	q.down(0)
	return first
}

// Push and Pop keep the small queue test API; the tokenizer uses typed methods above
// so the hot path never pays container/heap's interface boxing cost.
func (q *mergeQueue) Push(x any) {
	candidate, ok := x.(mergeCandidate)
	if !ok {
		panic("tokenestimate: invalid merge candidate")
	}
	q.push(candidate)
}

func (q *mergeQueue) Pop() any { return q.pop() }

type bpeSymbol struct{ id, prev, next int }

// Scratch is local to one count call, never shared across requests or retained globally.
type bpeScratch struct {
	symbols []bpeSymbol
	queue   mergeQueue
}

func (t *deepSeekTokenizer) countPiece(text string, scratch *bpeScratch) int {
	scratch.symbols = slices.Grow(scratch.symbols[:0], len(text))[:len(text)]
	symbols := scratch.symbols
	for i := range symbols {
		symbols[i] = bpeSymbol{id: t.byteID[text[i]], prev: i - 1, next: i + 1}
	}
	queue := scratch.queue[:0]
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
	for i := len(queue)/2 - 1; i >= 0; i-- {
		queue.down(i)
	}
	count := len(symbols)
	for len(queue) > 0 {
		c := queue.pop()
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
				queue.push(next)
			}
		}
	}
	scratch.queue = queue
	return count
}
