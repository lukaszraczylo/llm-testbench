package tests

import (
	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

// paperPQCompressionExcerpt is a ~143-word, technically accurate original
// description of product quantization written for this test, pinning the
// dimension, m, and nbits parameters needed to derive a compression ratio.
const paperPQCompressionExcerpt = `Product quantization compresses high-dimensional vectors for approximate
nearest-neighbor search by splitting each vector into m equal-length
subvectors, then quantizing each subvector independently against its own
codebook. Each codebook is trained offline with k-means and holds 2^nbits
centroids. Rather than storing the original subvector, the index stores only
the index of its nearest centroid: an nbits-bit code. Concatenating the m
per-subvector codes gives the full compressed representation of the vector.
A benchmark index in this report indexed 128-dimensional vectors stored as
32-bit floats, so one uncompressed vector occupies 128 times 4, or 512,
bytes. The index split each vector into m equal to 8 subvectors and used
nbits equal to 8 bits per subvector, giving each subquantizer's codebook 256
centroids. Search computes approximate distances directly against the
stored codes using precomputed per-centroid distance tables, never
decompressing a vector back to its original 512-byte form.`

// paperPQCompressionRatioWant is derived by calling wpQuantizedCodeBytes,
// not hardcoded.
//
// ground truth: code bytes = m*nbits/8 = 8*8/8 = 8 bytes per vector;
// compression ratio = raw bytes / code bytes = 512/8 = 64.
// whitepapers_vector_test.go independently recomputes this with plain
// arithmetic.
var paperPQCompressionRatioWant = 512 / wpQuantizedCodeBytes(8, 8)

// paperPQCompressionRatioTest: derive a product-quantization compression
// ratio from an inline excerpt's m/nbits/dimension parameters.
func paperPQCompressionRatioTest() testkit.Test {
	prompt := `Read this excerpt about product quantization (PQ) for vector search:

` + paperPQCompressionExcerpt + `

The benchmark index above stores each compressed vector as its m
per-subvector codes packed with no wasted bits. What is the compression
ratio of the compressed representation versus the original 512-byte
float32 vector (raw bytes divided by compressed bytes)? Respond with only
the number.`

	return testkit.Test{
		ID:          "paper-pq-compression-ratio",
		Category:    "research",
		Subcategory: "whitepapers",
		Description: "Derive a product-quantization compression ratio from an inline excerpt's m, nbits, and raw-dimension parameters.",
		Prompt:      prompt,
		Eval:        eval.Numeric(eval.ExtractLastNumber[int], paperPQCompressionRatioWant, 0),
	}
}

// paperBloomFPRateExcerpt is a ~157-word, technically accurate original
// description of the Bloom filter false-positive formula written for this
// test, pinning m, n, and k.
const paperBloomFPRateExcerpt = `A Bloom filter is a space-efficient structure for testing set membership
that trades a small, tunable false-positive rate for large memory savings,
and never produces a false negative. It holds an m-bit array, initially all
zero, and uses k independent hash functions. Inserting an element sets the
k bit positions its hashes point to; querying an element checks whether all
k of those positions are set, and if so reports "possibly present." The
false-positive probability after inserting n elements is well approximated
by p equals the quantity one minus e to the power of negative k times n
divided by m, that whole quantity raised to the power k. A caching layer
described in this report sized its filter at m equal to 10000 bits for n
equal to 1000 expected keys, and chose k equal to 7 hash functions,
following the standard guidance to pick k near m over n times the natural
log of 2.`

// paperBloomFPRateWant is derived by calling wpBloomFalsePositiveRate, not
// hardcoded.
//
// ground truth: p = (1 - e^(-k*n/m))^k with m=10000, n=1000, k=7.
// whitepapers_vector_test.go recomputes this independently with math.Pow
// and math.Exp inline.
var paperBloomFPRateWant = round4dp(wpBloomFalsePositiveRate(10000, 1000, 7))

// paperBloomFPRateTest: compute a Bloom filter's false-positive rate from
// an inline excerpt's formula and m/n/k parameters.
func paperBloomFPRateTest() testkit.Test {
	prompt := `Read this excerpt about Bloom filters:

` + paperBloomFPRateExcerpt + `

Using the exact formula given above and the caching layer's parameters
(m=10000, n=1000, k=7), compute the false-positive probability p. Respond
with only the number, rounded to 4 decimal places.`

	return testkit.Test{
		ID:          "paper-bloom-fp-rate",
		Category:    "research",
		Subcategory: "whitepapers",
		Description: "Compute a Bloom filter's false-positive rate from an inline excerpt's formula and m/n/k parameters.",
		Prompt:      prompt,
		Eval:        eval.Numeric(eval.ExtractLastNumber[float64], paperBloomFPRateWant, 0.0005),
	}
}

// paperTFIDFExcerpt is a ~148-word, technically accurate original
// description of the plain (unsmoothed) TF-IDF formula written for this
// test.
const paperTFIDFExcerpt = `TF-IDF scores how important a term is to one document within a corpus by
combining two signals: how often the term appears in that document, and how
rare the term is across the whole corpus. This report uses the plainest
textbook variant. Term frequency, tf(t,d), is simply the raw count of term
t in document d, with no normalization. Inverse document frequency,
idf(t), is the natural logarithm of N divided by df(t), where N is the
total number of documents in the corpus and df(t) is the number of
documents that contain t at least once. The TF-IDF weight of a term in a
document is the product tf(t,d) times idf(t): a term that appears often in
one document but rarely elsewhere scores highest, while a term appearing in
every document scores zero regardless of how often it repeats, since its
idf is ln(1) = 0 exactly.`

// paperTFIDFDoc1, paperTFIDFDoc2, and paperTFIDFDoc3 are the pre-tokenized
// (whitespace-split, lowercase) mini-corpus for paperTFIDFTermScoreTest.
var (
	paperTFIDFDoc1   = []string{"the", "cat", "sat", "on", "the", "mat"}
	paperTFIDFDoc2   = []string{"the", "dog", "sat", "on", "the", "log"}
	paperTFIDFDoc3   = []string{"cats", "and", "dogs", "are", "great", "pets"}
	paperTFIDFCorpus = [][]string{paperTFIDFDoc1, paperTFIDFDoc2, paperTFIDFDoc3}
)

// paperTFIDFTermScoreWant is derived by calling wpTermFrequency,
// wpDocumentFrequency, and wpTFIDF, not hardcoded.
//
// ground truth: tf("sat", doc1) = 1; df("sat") = 2 (doc1 and doc2 contain
// it, doc3 does not); N = 3; idf = ln(3/2); tfidf = 1 * ln(3/2).
// whitepapers_vector_test.go recomputes this independently with math.Log.
var paperTFIDFTermScoreWant = round4dp(wpTFIDF(
	wpTermFrequency("sat", paperTFIDFDoc1),
	wpDocumentFrequency("sat", paperTFIDFCorpus),
	len(paperTFIDFCorpus),
))

// paperTFIDFTermScoreTest: compute the TF-IDF weight of one term in one
// document of an inline 3-document mini-corpus, using the exact formula
// given in the excerpt.
func paperTFIDFTermScoreTest() testkit.Test {
	prompt := `Read this excerpt about TF-IDF:

` + paperTFIDFExcerpt + `

Here is a 3-document corpus:

Document 1: "the cat sat on the mat"
Document 2: "the dog sat on the log"
Document 3: "cats and dogs are great pets"

Using the exact formulas given above (tf = raw count of the term in the
document, idf = natural log of N divided by df), compute the TF-IDF weight
of the term "sat" in Document 1. Respond with only the number, rounded to
4 decimal places.`

	return testkit.Test{
		ID:          "paper-tfidf-term-score",
		Category:    "research",
		Subcategory: "whitepapers",
		Description: "Compute a term's TF-IDF weight in one document of an inline 3-document mini-corpus using a formula given in the excerpt.",
		Prompt:      prompt,
		Eval:        eval.Numeric(eval.ExtractLastNumber[float64], paperTFIDFTermScoreWant, 0.0005),
	}
}

// paperRecallExcerpt is a ~137-word, technically accurate original
// description of the recall@k retrieval metric written for this test.
const paperRecallExcerpt = `Recall@k is a standard retrieval-quality metric for evaluating a nearest-
neighbor or search system: for one query, it is the fraction of that
query's truly relevant items that appear anywhere within the system's top
k returned results, regardless of their exact rank inside that top-k list.
Formally, recall@k equals the size of the intersection between the
relevant set and the top-k retrieved set, divided by the size of the
relevant set. A perfect recall@k of 1.0 means every relevant item was
retrieved somewhere in the top k, even if none of them landed in rank one;
a lower score means some relevant items were pushed outside the top k
entirely and are effectively lost to any downstream ranking or re-ranking
stage, since a re-ranker can only reorder candidates that retrieval already
surfaced, never recover ones it dropped.`

// paperRecallRelevant and paperRecallRetrievedTop6 are the inline
// relevant-set and top-6 retrieved-ranking fixtures for
// paperRecallAtKTest.
//
// ground truth: relevant = {D2,D5,D7,D9} (size 4); of those, D2, D5, and
// D7 appear somewhere in the top-6 retrieved list, but D9 does not, so the
// intersection has size 3. recall@6 = |intersection| / |relevant| = 3/4 =
// 0.75. whitepapers_vector_test.go independently recomputes this by
// counting the intersection directly, not via wpRecallAtK.
var (
	paperRecallRelevant      = []string{"D2", "D5", "D7", "D9"}
	paperRecallRetrievedTop6 = []string{"D1", "D5", "D3", "D7", "D6", "D2"}
	paperRecallAtKWant       = round4dp(wpRecallAtK(paperRecallRelevant, paperRecallRetrievedTop6))
)

// paperRecallAtKTest: compute recall@k from an inline relevant-item set and
// a ranked top-k retrieval list.
func paperRecallAtKTest() testkit.Test {
	prompt := `Read this excerpt about the recall@k retrieval metric:

` + paperRecallExcerpt + `

A query's truly relevant documents are: D2, D5, D7, D9.

A retrieval system's ranked top-6 results for that query are, in rank
order: D1, D5, D3, D7, D6, D2.

Using the definition given above, compute recall@6 for this query. Respond
with only the number, as a decimal (e.g. 0.5), rounded to 4 decimal
places.`

	return testkit.Test{
		ID:          "paper-recall-at-k",
		Category:    "research",
		Subcategory: "whitepapers",
		Description: "Compute recall@k from an inline relevant-item set and a ranked top-k retrieval list.",
		Prompt:      prompt,
		Eval:        eval.Numeric(eval.ExtractLastNumber[float64], paperRecallAtKWant, 0.0005),
	}
}
