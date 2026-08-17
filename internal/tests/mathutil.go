package tests

import "math"

// cosineSimilarity computes the cosine similarity of two equal-length
// vectors: dot(a,b) / (|a| * |b|). Used to derive ground-truth expected
// values for numeric tests at catalog-registration time rather than
// hardcoding a pre-computed constant.
func cosineSimilarity(a, b []float64) float64 {
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// round4dp rounds v to 4 decimal places, matching the precision the
// py-cosine prompt asks the model to answer to.
func round4dp(v float64) float64 {
	return math.Round(v*10000) / 10000
}
