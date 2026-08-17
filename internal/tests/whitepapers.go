package tests

import (
	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

func registerWhitepapersTests(r *testkit.Registry) {
	r.Register(paperHnswParamsTest())
	r.Register(paperPQCompressionRatioTest())
	r.Register(paperBloomFPRateTest())
	r.Register(paperTFIDFTermScoreTest())
	r.Register(paperRecallAtKTest())
	r.Register(paperLSMWriteAmplificationTest())
	r.Register(paperRaftQuorumFailuresTest())
	r.Register(paperBTreeHeightLevelsTest())
	r.Register(paperCAPAvailabilityChoiceTest())
	r.Register(paperAttentionScaleFactorTest())
}

// paperHnswParamsExcerpt is a ~150-word, technically accurate original
// description of HNSW's three tuning parameters, written for this test
// (not reproduced from any paper), pinning three specific numeric values.
//
// S11: the sentence about M was reworded. The original phrasing ("M=16,
// giving each vector roughly 32 edges once both directions are counted")
// conflated M with total node degree and misattributed the factor of two
// to link bidirectionality. HNSW actually permits the base layer (layer 0)
// twice as many links as upper layers (Mmax0 = 2*M) to keep that most
// heavily traversed layer well connected - the excerpt now states that
// correctly, still pinning M=16 as the single unambiguous answer to
// question 1.
const paperHnswParamsExcerpt = `HNSW (Hierarchical Navigable Small World) builds a
multi-layer proximity graph over the indexed vectors, where each upper
layer is a sparser sub-mesh of the layer below it. Search descends from a
coarse entry point at the top layer down to a precise result at the base
layer. Three parameters dominate the accuracy, speed, and memory trade-off.
M controls the maximum number of bidirectional links each node keeps per
layer; a benchmark index described in this report used M=16 links per node
on the upper layers, with the base layer permitted twice that (Mmax0 = 32)
to stay well connected. efConstruction bounds the size of the dynamic
candidate list explored while inserting a new node, and was set to 200
during the build - higher values slow indexing but produce a
better-connected graph. efSearch, set to 64 at query time, bounds the same
candidate list during search; raising it improves recall at the cost of
latency, independent of the build-time efConstruction value.`

// paperHnswParamsTest: extract three precise numeric parameters from a
// technical excerpt.
//
// ground truth: the excerpt states M=16, efConstruction=200, efSearch=64
// verbatim; these are the only numbers attached to those parameter names in
// the text.
func paperHnswParamsTest() testkit.Test {
	prompt := `Read this excerpt about HNSW indexing:

` + paperHnswParamsExcerpt + `

Answer three questions based only on this excerpt:
1. What value of M did the benchmark index use?
2. What value of efConstruction was used during the index build?
3. What value of efSearch was used at query time?

Respond with only a JSON object: {"M":<number>,"efConstruction":<number>,"efSearch":<number>}`

	evaluator := eval.Mean(
		eval.JSONField("M", 16),
		eval.JSONField("efConstruction", 200),
		eval.JSONField("efSearch", 64),
	)

	return testkit.Test{
		ID:          "paper-hnsw-params",
		Category:    "research",
		Subcategory: "whitepapers",
		Description: "Extract three precise numeric HNSW tuning parameters from a technical excerpt.",
		Prompt:      prompt,
		MaxTokens:   300,
		Eval:        evaluator,
	}
}
