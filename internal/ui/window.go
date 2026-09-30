package ui

import "fmt"

// window cuts ls to at most budget lines, keeping lines [from, to) in view
// and marking how many lines were cut off each end. A budget under 3 (the
// terminal size is not known yet) or one that fits every line keeps them all.
func window(ls []string, from, to, budget int) []string {
	if budget < 3 || len(ls) <= budget {
		return ls
	}
	visible := budget - 2 // room for the ↑ and ↓ markers
	from, to = max(from, 0), max(to, from+1)
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
