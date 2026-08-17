package tests

import (
	"math"
	"strings"
)

// wpQuantizedCodeBytes returns the number of bytes needed to store one
// product-quantization code: m subvectors, each an nbits-bit centroid
// index, packed with no wasted bits. It assumes m*nbits is a multiple of
// 8, true for every configuration used in this catalog.
func wpQuantizedCodeBytes(m, nbits int) int {
	return m * nbits / 8
}

// wpBloomFalsePositiveRate computes the standard Bloom filter
// false-positive probability p = (1 - e^(-k*n/m))^k for an m-bit filter
// holding n inserted elements and k hash functions.
func wpBloomFalsePositiveRate(m, n, k int) float64 {
	exponent := -float64(k) * float64(n) / float64(m)
	return math.Pow(1-math.Exp(exponent), float64(k))
}

// wpTermFrequency counts how many times term appears in doc, splitting on
// whitespace (doc is pre-tokenized) and comparing case-insensitively.
func wpTermFrequency(term string, doc []string) int {
	count := 0
	for _, w := range doc {
		if strings.EqualFold(w, term) {
			count++
		}
	}
	return count
}

// wpDocumentFrequency counts how many of docs contain term at least once.
func wpDocumentFrequency(term string, docs [][]string) int {
	count := 0
	for _, doc := range docs {
		if wpTermFrequency(term, doc) > 0 {
			count++
		}
	}
	return count
}

// wpTFIDF computes the plain tf * ln(N/df) weight: tf is the raw term
// count in one document, df is the number of documents containing the
// term, and n is the total document count. No smoothing is applied,
// matching the formula stated in the wp-tfidf-term-score excerpt.
func wpTFIDF(tf, df, n int) float64 {
	return float64(tf) * math.Log(float64(n)/float64(df))
}

// wpRecallAtK computes the standard recall@k retrieval metric: the
// fraction of relevant that appears anywhere in retrievedTopK, regardless
// of rank.
func wpRecallAtK(relevant, retrievedTopK []string) float64 {
	retrievedSet := make(map[string]struct{}, len(retrievedTopK))
	for _, id := range retrievedTopK {
		retrievedSet[id] = struct{}{}
	}
	hit := 0
	for _, id := range relevant {
		if _, ok := retrievedSet[id]; ok {
			hit++
		}
	}
	return float64(hit) / float64(len(relevant))
}

// wpLSMWriteAmplification returns the write amplification factor for a
// byte that is rewritten rewriteCount times by compaction after its
// original flush write: the original write plus every rewrite, divided by
// the single original write.
func wpLSMWriteAmplification(rewriteCount int) int {
	return 1 + rewriteCount
}

// wpRaftQuorumSize returns the minimum number of votes/acknowledgments
// needed for a strict majority of an n-node Raft cluster: floor(n/2)+1.
func wpRaftQuorumSize(n int) int {
	return n/2 + 1
}

// wpRaftMaxTolerableFailures returns how many simultaneous node failures
// an n-node Raft cluster can sustain while a majority quorum, sized
// relative to the full n-node membership, remains reachable.
func wpRaftMaxTolerableFailures(n int) int {
	return n - wpRaftQuorumSize(n)
}

// wpBTreeMinBranchingLevels returns the smallest h (root counted as h=0)
// such that a tree with minimum fanout f can index at least n leaf key
// slots, per the f^(h+1) >= n bound stated in the wp-btree-height-levels
// excerpt.
func wpBTreeMinBranchingLevels(f, n int) int {
	h := 0
	capacity := f
	for capacity < n {
		capacity *= f
		h++
	}
	return h
}
