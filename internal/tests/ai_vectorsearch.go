package tests

import (
	"strings"

	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

func registerAIVectorSearchTests(r *testkit.Registry) {
	r.Register(vecCosineVsDotTest())
	r.Register(vecRecallAtKTest())
	r.Register(vecHNSWEfSearchTradeoffTest())
	r.Register(vecPQMemoryMathTest())
	r.Register(vecPreVsPostFilteringTest())
	r.Register(vecRRFFusionTest())
	r.Register(vecDistanceToSimilarityTest())
	r.Register(vecNearDuplicateThresholdTest())
	r.Register(vecIndexBuildQueryTradeoffTest())
	r.Register(vecEmbeddingDimensionTradeoffTest())
}

// aiRRFScore computes the standard Reciprocal Rank Fusion score for one
// document: the sum of 1/(k+rank) over every ranked list the document
// appears in, using 1-indexed ranks. A document absent from a list
// contributes nothing for that list (omit its rank from ranks).
func aiRRFScore(k int, ranks ...int) float64 {
	var sum float64
	for _, rank := range ranks {
		sum += 1 / float64(k+rank)
	}
	return sum
}

// aiUnitDistanceToCosineSimilarity converts a Euclidean distance d between
// two unit-length (L2-normalized) vectors to their cosine similarity, using
// ||a-b||^2 = |a|^2 + |b|^2 - 2*cos_sim = 2 - 2*cos_sim for |a|=|b|=1, so
// cos_sim = 1 - d^2/2.
func aiUnitDistanceToCosineSimilarity(d float64) float64 {
	return 1 - d*d/2
}

// vecCosineVsDotTest: confirm that for two already-unit-normalized vectors,
// the dot product equals their cosine similarity.
//
// ground truth: cosine similarity is dot(a,b) / (|a|*|b|). a = [0.6, 0.8]
// has |a| = sqrt(0.36+0.64) = sqrt(1) = 1, and b = [0.8, 0.6] has the same
// magnitude by symmetry. With both magnitudes exactly 1, the denominator is
// 1, so dot(a,b) and cosine similarity are the same value. Answer: yes.
func vecCosineVsDotTest() testkit.Test {
	prompt := `Two vectors have already been L2-normalized to unit length
(magnitude exactly 1): a = [0.6, 0.8] and b = [0.8, 0.6].

Confirm the magnitudes: |a| = sqrt(0.6^2 + 0.8^2) = sqrt(0.36 + 0.64) =
sqrt(1.0) = 1, and by the same arithmetic |b| = sqrt(0.8^2 + 0.6^2) = 1.

Cosine similarity is defined as dot(a,b) / (|a| * |b|). Since |a| and |b|
are both exactly 1 for this pair, is the plain dot product dot(a,b)
mathematically equivalent to the cosine similarity, with no further
division needed? Respond with only one word: yes or no.`

	return testkit.Test{
		ID:          "vec-cosine-vs-dot",
		Category:    "ai",
		Subcategory: "vector-search",
		Description: "Confirm that for two unit-normalized vectors, the plain dot product equals the cosine similarity.",
		Prompt:      prompt,
		Eval:        eval.Equals("yes"),
	}
}

// vecRecallAtKRelevant and vecRecallAtKRetrieved are the inline
// relevant-item set and ranked top-5 retrieval list for vecRecallAtKTest -
// a fresh fixture (V-prefixed ids, recall@5 not @6) distinct from
// paper-recall-at-k in whitepapers_vector.go.
var (
	vecRecallAtKRelevant  = []string{"V3", "V8", "V11", "V14", "V20"}
	vecRecallAtKRetrieved = []string{"V8", "V1", "V14", "V6", "V9"}
)

// vecRecallAtKWant is derived by calling wpRecallAtK (already defined in
// whitepapers_compute.go for the same recall@k metric), not hardcoded.
//
// ground truth: of the 5 relevant items (V3, V8, V11, V14, V20), exactly 2
// (V8 and V14) appear in the top-5 retrieved list; recall@5 = 2/5 = 0.4.
// ai_vectorsearch_test.go independently recomputes this with a plain set
// intersection.
var vecRecallAtKWant = wpRecallAtK(vecRecallAtKRelevant, vecRecallAtKRetrieved)

// vecRecallAtKTest: compute recall@k from a fresh relevant-item set and
// ranked top-k retrieval list, distinct from the whitepapers recall@k
// fixture.
func vecRecallAtKTest() testkit.Test {
	prompt := `Recall@k is a retrieval-quality metric: for one query, it is
the fraction of that query's truly relevant items that appear anywhere
within the system's top k retrieved results, regardless of exact rank.
recall@k = |relevant intersect top-k| / |relevant|.

A query's truly relevant vectors are: V3, V8, V11, V14, V20.

A vector search system's ranked top-5 results for that query are, in rank
order: V8, V1, V14, V6, V9.

Compute recall@5 for this query. Respond with only the number, as a
decimal (e.g. 0.6), rounded to 4 decimal places.`

	return testkit.Test{
		ID:          "vec-recall-at-k",
		Category:    "ai",
		Subcategory: "vector-search",
		Description: "Compute recall@5 from a fresh relevant-item set and ranked top-5 retrieval list.",
		Prompt:      prompt,
		Eval:        eval.Numeric(eval.ExtractLastNumber[float64], vecRecallAtKWant, 0.0005),
	}
}

// vecHNSWEfSearchTradeoffTest: name the shared direction recall and query
// latency move in when efSearch increases.
//
// ground truth: increasing efSearch (candidates explored per query) at
// fixed efConstruction and M makes the search examine more candidate nodes
// before stopping. That means more distance computations per query
// (latency up) and a lower chance of missing a true nearest neighbor
// (recall up) - both move in the same direction: up.
func vecHNSWEfSearchTradeoffTest() testkit.Test {
	prompt := `In an HNSW approximate nearest-neighbor index, efSearch
controls how many candidate nodes the query-time search explores before
stopping; efConstruction and M are fixed, index-build-time parameters that
do not change per query.

If you increase efSearch, keeping efConstruction and M fixed, query recall
and query latency move in the same direction as each other. Do they both
go up, or both go down? Respond with only one word: up or down.`

	return testkit.Test{
		ID:          "vec-hnsw-efsearch-tradeoff",
		Category:    "ai",
		Subcategory: "vector-search",
		Description: "State that increasing HNSW's efSearch moves recall and query latency in the same direction (up).",
		Prompt:      prompt,
		Eval:        eval.Equals("up"),
	}
}

// vecPQMemoryMathTest: derive total compressed index memory from PQ
// parameters distinct from the whitepapers PQ-compression-ratio fixture
// (different m, nbits, dimension, and a total-bytes framing rather than a
// ratio).
//
// ground truth: codeBytes = m*nbits/8 = 16*4/8 = 8 bytes per compressed
// vector (via wpQuantizedCodeBytes, already defined in
// whitepapers_compute.go). Total for 1,000,000 vectors = 8 * 1,000,000 =
// 8,000,000 bytes. ai_vectorsearch_test.go independently recomputes this
// with plain arithmetic.
var vecPQMemoryMathWant = wpQuantizedCodeBytes(16, 4) * 1000000

func vecPQMemoryMathTest() testkit.Test {
	prompt := `Product quantization (PQ) compresses each vector by splitting
it into m equal-length subvectors and quantizing each one independently
against its own codebook, storing only each subvector's nbits-bit centroid
index. Concatenating the m per-subvector codes gives the full compressed
representation, with no wasted bits (assume m*nbits is a multiple of 8).

An index uses m = 16 subvectors and nbits = 4 bits per subvector.

What is the total memory, in bytes, needed to store the compressed
representations of 1,000,000 vectors indexed with these PQ parameters?
Respond with only the integer number of bytes, with no commas, units, or
other text (for example: 12345678).`

	return testkit.Test{
		ID:          "vec-pq-memory-math",
		Category:    "ai",
		Subcategory: "vector-search",
		Description: "Derive total compressed-index memory in bytes for 1M vectors from PQ parameters m=16, nbits=4.",
		Prompt:      prompt,
		Eval:        eval.Numeric(eval.ExtractLastNumber[int], vecPQMemoryMathWant, 0),
	}
}

// vecPreVsPostFilteringTest: choose pre-filtering over post-filtering when
// a metadata filter is highly selective relative to the requested result
// count.
//
// ground truth: the filter matches only 0.5% of 10 million vectors (about
// 50,000 candidates). Post-filtering runs ANN search first and discards
// non-matching results afterward; with such low selectivity, a top-k ANN
// search is very likely to return few or zero matching candidates unless k
// is made enormous, so it cannot reliably return 10 matching results.
// Pre-filtering (restricting the searched candidate set to only
// filter-matching vectors before or during the ANN search) is required to
// reliably surface 10 results that satisfy the filter.
func vecPreVsPostFilteringTest() testkit.Test {
	prompt := `An ANN vector index holds 10,000,000 vectors. A metadata
filter (e.g. tenant_id = X) matches only 0.5% of those vectors. A query
needs the top 10 nearest-neighbor results that also satisfy this filter.

Should the filter be applied before/during the ANN search so only
filter-matching vectors are ever considered (pre-filtering), or should the
ANN search run first and non-matching results be discarded afterward
(post-filtering), to reliably return 10 matching results? Respond with
only one word: pre or post.`

	return testkit.Test{
		ID:          "vec-pre-vs-post-filtering",
		Category:    "ai",
		Subcategory: "vector-search",
		Description: "Choose pre-filtering over post-filtering ANN search when the metadata filter is highly selective (0.5%).",
		Prompt:      prompt,
		Eval:        eval.Equals("pre"),
	}
}

// vecRRFFusionKeywordRanks and vecRRFFusionVectorRanks are the inline,
// 1-indexed rank lists for vecRRFFusionTest, also used to build the
// prompt's rank-list text via strings.Join so the displayed lists and the
// computed ground truth can never drift apart. D5 sits at rank 3 in the
// keyword list and rank 7 in the vector list.
var (
	vecRRFFusionKeywordRanks = []string{"D2", "D9", "D5", "D1", "D7", "D3", "D6", "D4"}
	vecRRFFusionVectorRanks  = []string{"D9", "D1", "D3", "D2", "D8", "D6", "D5", "D4"}
)

// vecRRFFusionK is the RRF constant used in vecRRFFusionTest's prompt,
// stated explicitly so the answer is computable rather than memorized.
const vecRRFFusionK = 60

// vecRRFFusionDoc is the document vecRRFFusionTest asks the model to score.
const vecRRFFusionDoc = "D5"

// aiRankOf returns id's 1-indexed position in ranked, or 0 if id is
// absent (matching aiRRFScore's convention that an absent document
// contributes nothing for that list).
func aiRankOf(id string, ranked []string) int {
	for i, v := range ranked {
		if v == id {
			return i + 1
		}
	}
	return 0
}

// vecRRFFusionWant is derived by calling aiRRFScore with vecRRFFusionDoc's
// actual ranks in both lists, looked up via aiRankOf rather than
// hardcoded.
//
// ground truth: RRF(d) = sum over each list of 1/(k + rank_list(d)). D5 is
// rank 3 in the keyword list and rank 7 in the vector list, so RRF(D5) =
// 1/(60+3) + 1/(60+7) = 1/63 + 1/67 ~= 0.030798. Rounded to 4dp: 0.0308.
// ai_vectorsearch_test.go independently recomputes this with plain
// arithmetic.
var vecRRFFusionWant = round4dp(aiRRFScore(
	vecRRFFusionK,
	aiRankOf(vecRRFFusionDoc, vecRRFFusionKeywordRanks),
	aiRankOf(vecRRFFusionDoc, vecRRFFusionVectorRanks),
))

func vecRRFFusionTest() testkit.Test {
	prompt := `Reciprocal Rank Fusion (RRF) combines multiple ranked result
lists into one fused score per document. For a document d, its RRF score is
the sum, over every ranked list it appears in, of 1/(k + rank), where rank
is the document's 1-indexed position in that list and k is a fixed
constant. A document missing from a list contributes nothing for that list.
Use k = 60.

Keyword search results, ranked 1st to 8th: ` + strings.Join(vecRRFFusionKeywordRanks, ", ") + `

Vector search results, ranked 1st to 8th: ` + strings.Join(vecRRFFusionVectorRanks, ", ") + `

Using the formula above with k = 60, compute the RRF score for document ` + vecRRFFusionDoc + `.
Respond with only the number, rounded to 4 decimal places.`

	return testkit.Test{
		ID:          "vec-rrf-fusion",
		Category:    "ai",
		Subcategory: "vector-search",
		Description: "Compute a document's Reciprocal Rank Fusion score (k=60) from its ranks in two inline result lists.",
		Prompt:      prompt,
		Eval:        eval.Numeric(eval.ExtractLastNumber[float64], vecRRFFusionWant, 0.0005),
	}
}

// vecDistanceToSimilarityWant is derived by calling
// aiUnitDistanceToCosineSimilarity, not hardcoded.
//
// ground truth: for unit-length vectors, ||a-b||^2 = 2 - 2*cos_sim, so
// cos_sim = 1 - d^2/2. With d = 0.6, d^2 = 0.36, cos_sim = 1 - 0.18 = 0.82.
// ai_vectorsearch_test.go independently recomputes this with plain
// arithmetic.
var vecDistanceToSimilarityWant = round4dp(aiUnitDistanceToCosineSimilarity(0.6))

func vecDistanceToSimilarityTest() testkit.Test {
	prompt := `Two vectors are both unit-length (L2-normalized to magnitude
1). For unit-length vectors, squared Euclidean distance relates to cosine
similarity by: ||a-b||^2 = |a|^2 + |b|^2 - 2*cos_sim(a,b) = 2 - 2*cos_sim(a,b),
since |a|=|b|=1. Rearranging: cos_sim(a,b) = 1 - (||a-b||^2) / 2.

The Euclidean distance between this pair is 0.6.

Using the formula above, compute cos_sim(a,b). Respond with only the
number, rounded to 4 decimal places.`

	return testkit.Test{
		ID:          "vec-distance-to-similarity",
		Category:    "ai",
		Subcategory: "vector-search",
		Description: "Convert a Euclidean distance between unit vectors to cosine similarity using the stated formula.",
		Prompt:      prompt,
		Eval:        eval.Numeric(eval.ExtractLastNumber[float64], vecDistanceToSimilarityWant, 0.0005),
	}
}

// vecNearDuplicateA and vecNearDuplicateB are the inline 4-dim vectors for
// vecNearDuplicateThresholdTest, distinct from py-cosine's 8-dim fixture in
// python.go.
var (
	vecNearDuplicateA = []float64{1, 2, 2, 1}
	vecNearDuplicateB = []float64{2, 2, 1, 1}
)

// vecNearDuplicateThreshold is the near-duplicate cosine similarity cutoff
// stated in vecNearDuplicateThresholdTest's prompt.
const vecNearDuplicateThreshold = 0.97

// vecNearDuplicateThresholdWant is derived by calling cosineSimilarity
// (mathutil.go), not hardcoded.
//
// ground truth: dot(a,b) = 1*2+2*2+2*1+1*1 = 9; |a|^2 = 1+4+4+1 = 10;
// |b|^2 = 4+4+1+1 = 10; cosine similarity = 9/sqrt(10*10) = 9/10 = 0.9.
// 0.9 - 0.97 = -0.07. ai_vectorsearch_test.go independently recomputes
// this with plain arithmetic.
var vecNearDuplicateThresholdWant = round4dp(cosineSimilarity(vecNearDuplicateA, vecNearDuplicateB) - vecNearDuplicateThreshold)

func vecNearDuplicateThresholdTest() testkit.Test {
	prompt := `A vector index deduplicates near-identical items: a candidate
pair is treated as a near-duplicate only when its cosine similarity is at
or above 0.97.

Candidate pair: a = [1, 2, 2, 1], b = [2, 2, 1, 1].

Compute the cosine similarity of a and b, then subtract the 0.97 threshold
from it (similarity minus threshold). Respond with only the resulting
number, rounded to 4 decimal places (it may be negative).`

	return testkit.Test{
		ID:          "vec-near-duplicate-threshold",
		Category:    "ai",
		Subcategory: "vector-search",
		Description: "Compute cosine similarity of a 4-dim vector pair and its signed distance from the 0.97 near-duplicate threshold.",
		Prompt:      prompt,
		Eval:        eval.Numeric(eval.ExtractLastNumber[float64], vecNearDuplicateThresholdWant, 0.0005),
	}
}

// vecIndexBuildQueryTradeoffTest: pick the index type each stated
// constraint forces, between HNSW (slower build, faster/more accurate
// query) and IVF-Flat with few clusters (fast build, coarser query).
//
// ground truth: scenario_a's constraint - 50M vectors must be indexed in
// under 10 minutes, with query recall/latency explicitly not critical -
// rules out HNSW's comparatively slow graph-construction build and forces
// IVF-Flat's fast single-pass clustering build. scenario_b's constraint -
// query recall is critical, build time is unconstrained - forces HNSW,
// since its graph gives materially better recall at query time than a
// coarse IVF-Flat partition.
func vecIndexBuildQueryTradeoffTest() testkit.Test {
	prompt := `Two ANN index types are available:
- hnsw: builds a navigable graph; build time is comparatively slow, but
  queries are fast and highly accurate (high recall).
- ivf-flat: clusters vectors into a small number of buckets via k-means;
  build time is comparatively fast, but queries are coarser (lower recall)
  than a well-tuned HNSW graph.

scenario_a: "You must index 50,000,000 vectors in under 10 minutes on
commodity hardware. Query recall and query latency are not critical for
this use case."

scenario_b: "Query recall is critical for this use case and build time is
not constrained at all."

For each scenario, which index type must you choose? Respond with only a
JSON object: {"scenario_a":"hnsw"|"ivf-flat","scenario_b":"hnsw"|"ivf-flat"}`

	evaluator := eval.Mean(
		eval.JSONField("scenario_a", "ivf-flat"),
		eval.JSONField("scenario_b", "hnsw"),
	)

	return testkit.Test{
		ID:          "vec-index-build-query-tradeoff",
		Category:    "ai",
		Subcategory: "vector-search",
		Description: "Pick HNSW vs IVF-Flat for a build-time-constrained scenario and a recall-critical scenario.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// vecEmbeddingDimensionTradeoffTest: pick the embedding dimension that fits
// a fixed, uncompressed float32 storage budget.
//
// ground truth: bytes per vector = dims * 4 (float32). 128-dim: 128*4 = 512
// bytes/vector, total for 100,000,000 vectors = 51,200,000,000 bytes =
// 51.2 GB (using decimal GB = 1e9 bytes), which fits the 60 GB budget.
// 768-dim: 768*4 = 3072 bytes/vector, total = 307,200,000,000 bytes =
// 307.2 GB, which does not fit.
func vecEmbeddingDimensionTradeoffTest() testkit.Test {
	prompt := `You must store float32 (4 bytes per dimension), uncompressed
embeddings for 100,000,000 vectors, with a hard storage budget of 60 GB
(1 GB = 1,000,000,000 bytes). No quantization or compression is available.

Two embedding models are candidates:
- option 128: produces 128-dimensional embeddings.
- option 768: produces 768-dimensional embeddings.

Which option's total uncompressed storage fits within the 60 GB budget?
Respond with only a JSON object: {"choice":"128"|"768"}`

	return testkit.Test{
		ID:          "vec-embedding-dimension-tradeoff",
		Category:    "ai",
		Subcategory: "vector-search",
		Description: "Pick the embedding dimension (128 vs 768) whose uncompressed float32 storage fits a fixed 60 GB budget for 100M vectors.",
		Prompt:      prompt,
		Eval:        eval.JSONField("choice", "128"),
	}
}
