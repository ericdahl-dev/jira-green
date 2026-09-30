package ui

import "fmt"

// fit cuts a screen to height lines. The tail (a status or key hint line)
// is kept first, then the head (a pinned header), and body scrolls in what
// is left, keeping body lines [from, to) in view. A height of 0 means the
// terminal size is not known yet, so everything is kept.
func fit(head, body, tail []string, from, to, height int) []string {
	if height <= 0 {
		return append(append(append([]string{}, head...), body...), tail...)
	}
	tail = tail[max(len(tail)-height, 0):]
	head = head[:min(len(head), height-len(tail))]
	out := append([]string{}, head...)
	out = append(out, window(body, from, to, height-len(tail)-len(head))...)
	return append(out, tail...)
}

// window cuts ls to at most budget lines, keeping lines [from, to) in view.
// With room for them (a budget of 3 or more) it marks how many lines were
// cut off each end; a smaller budget shows only the lines around from.
func window(ls []string, from, to, budget int) []string {
	if len(ls) <= budget {
		return ls
	}
	if budget <= 0 {
		return nil
	}
	from, to = max(from, 0), max(to, from+1)
	if budget < 3 {
		start := min(max(from, 0), len(ls)-budget)
		return ls[start : start+budget]
	}
	visible := budget - 2 // room for the ↑ and ↓ markers
	start := from - max(visible-(to-from), 0)/2
	start = min(max(start, 0), len(ls)-visible)
	end := start + visible

	out := make([]string, 0, budget)
	if start > 0 {
		out = append(out, dimStyle.Render(fmt.Sprintf("  ↑ %d lines above", start)))
	}
	out = append(out, ls[start:end]...)
	if end < len(ls) {
		out = append(out, dimStyle.Render(fmt.Sprintf("  ↓ %d lines below", len(ls)-end)))
	}
	return out
}
