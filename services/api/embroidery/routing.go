package embroidery

import "sort"

// routePlan preserves first-use thread order to avoid surprise colour changes,
// then selects the nearest available block within each thread group.
func routePlan(plan []Block) []Block {
	if len(plan) < 2 {
		return plan
	}
	order := []string{}
	groups := map[string][]Block{}
	for _, b := range plan {
		if _, ok := groups[b.ThreadID]; !ok {
			order = append(order, b.ThreadID)
		}
		groups[b.ThreadID] = append(groups[b.ThreadID], b)
	}
	result := make([]Block, 0, len(plan))
	current := Point{}
	for _, thread := range order {
		remaining := groups[thread]
		for len(remaining) > 0 {
			sort.SliceStable(remaining, func(i, j int) bool {
				di, dj := distance(current, remaining[i].Entry), distance(current, remaining[j].Entry)
				if di == dj {
					return remaining[i].ID < remaining[j].ID
				}
				return di < dj
			})
			chosen := remaining[0]
			result = append(result, chosen)
			current = chosen.Exit
			remaining = remaining[1:]
		}
	}
	return result
}
