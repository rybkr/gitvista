// Package cli provides a lightweight CLI framework with colored help,
// subcommand dispatch, and "did you mean?" suggestions.
package cli

// Suggest returns the best matching candidate for input, or "" if no
// candidate is within the edit distance threshold max(2, len(input)/3).
func Suggest(input string, candidates []string) string {
	if input == "" {
		return ""
	}

	threshold := max(2, len(input)/3)

	best := ""
	bestDist := threshold + 1

	for _, c := range candidates {
		d := levenshtein(input, c)
		if d < bestDist {
			bestDist = d
			best = c
		}
	}

	return best
}

// levenshtein computes the Levenshtein (edit) distance between two strings
// using a single-row dynamic programming approach.
func levenshtein(a, b string) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	// Use shorter string for the row to save memory.
	if len(a) > len(b) {
		a, b = b, a
	}

	row := make([]int, len(a)+1)
	for i := range row {
		row[i] = i
	}

	for j := 1; j <= len(b); j++ {
		prev := row[0]
		row[0] = j
		for i := 1; i <= len(a); i++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			tmp := row[i]
			// min of deletion, insertion, substitution
			row[i] = min(row[i]+1, min(row[i-1]+1, prev+cost))
			prev = tmp
		}
	}

	return row[len(a)]
}
