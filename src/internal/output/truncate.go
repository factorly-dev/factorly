// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package output

import "fmt"

// Truncate limits s to maxBytes using a 60% head + 40% tail strategy.
// If s fits within maxBytes, it is returned unchanged.
// Returns the truncated string with a marker showing how many bytes were removed.
func Truncate(s string, maxBytes int) string {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s
	}

	// Compute marker length for the number of bytes we'll remove.
	// We need to estimate first, then adjust.
	removed := len(s) - maxBytes
	marker := fmt.Sprintf("\n\n[... truncated %d bytes ...]\n\n", removed)

	// If maxBytes is too small to fit marker + any content, just return prefix.
	if maxBytes <= len(marker) {
		return s[:maxBytes]
	}

	available := maxBytes - len(marker)
	headSize := available * 60 / 100
	tailSize := available - headSize

	// Recompute marker with exact removed byte count.
	actualRemoved := len(s) - headSize - tailSize
	marker = fmt.Sprintf("\n\n[... truncated %d bytes ...]\n\n", actualRemoved)

	// Adjust if marker length changed (digit count difference).
	for headSize+len(marker)+tailSize > maxBytes {
		// Shrink head by 1 to compensate.
		headSize--
		if headSize < 0 {
			headSize = 0
		}
		actualRemoved = len(s) - headSize - tailSize
		marker = fmt.Sprintf("\n\n[... truncated %d bytes ...]\n\n", actualRemoved)
	}

	return s[:headSize] + marker + s[len(s)-tailSize:]
}
