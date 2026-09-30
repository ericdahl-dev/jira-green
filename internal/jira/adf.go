package jira

// mentions walks an Atlassian Document Format tree and returns every
// mentioned account ID. Mentions without an ID are skipped.
func mentions(node any) []string {
	var out []string
	var walk func(any)
	walk = func(n any) {
		switch v := n.(type) {
		case map[string]any:
			if v["type"] == "mention" {
				if attrs, ok := v["attrs"].(map[string]any); ok {
					if id, _ := attrs["id"].(string); id != "" {
						out = append(out, id)
					}
				}
			}
			walk(v["content"])
		case []any:
			for _, c := range v {
				walk(c)
			}
		}
	}
	walk(node)
	return out
}
