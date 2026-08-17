package tests

import (
	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

func registerAIRAGTests(r *testkit.Registry) {
	r.Register(ragChunkSizeTradeoffTest())
	r.Register(ragRerankerPlacementTest())
	r.Register(ragRetrievalFailureModeTest())
	r.Register(ragCitationGroundingTest())
	r.Register(ragContextSelectionVsStuffingTest())
	r.Register(ragIndexStalenessTest())
	r.Register(ragEvalMetricChoiceTest())
	r.Register(ragHallucinationMitigationOrderingTest())
	r.Register(ragMultihopDecompositionTest())
	r.Register(ragPreAssemblyDedupTest())
}

// ragChunkSizeTradeoffTest: pick the smallest chunk size that reliably
// holds a full fact without splitting it, given an explicit token-span
// constraint.
//
// ground truth: facts span up to 450 tokens. The 128-token option is
// smaller than 450, so it risks splitting a fact across two chunks. Both
// the 512-token and 2048-token options are large enough to hold a 450-
// token fact whole, but the larger option dilutes the embedded chunk with
// more unrelated surrounding text. The smallest viable (non-splitting)
// option is 512.
func ragChunkSizeTradeoffTest() testkit.Test {
	prompt := `You are choosing a chunk size for a RAG index over dense
technical reference documents. A single self-contained fact in these
documents commonly spans up to 450 tokens; a chunk boundary that falls
inside a fact's span can leave a retrieved chunk missing part of that
fact. A chunk much larger than a fact's span dilutes the embedding with
unrelated surrounding text, lowering retrieval precision for queries about
that fact.

Three chunk-size options are available: 128 tokens, 512 tokens, and 2048
tokens.

Which of these is the smallest option that reliably holds a full 450-token
fact without ever splitting it across two chunks? Respond with only a
JSON object: {"chunk_size_tokens":128|512|2048}`

	return testkit.Test{
		ID:          "rag-chunk-size-tradeoff",
		Category:    "ai",
		Subcategory: "rag",
		Description: "Pick the smallest chunk size (512) that reliably holds a 450-token fact whole without excess dilution.",
		Prompt:      prompt,
		Eval:        eval.JSONField("chunk_size_tokens", 512),
	}
}

// ragRerankerPlacementTest: order the three RAG pipeline stages -
// retrieve, rerank, generate.
//
// ground truth: retrieval must run first to produce a broad candidate set
// from the index; a reranker (e.g. a cross-encoder) can only reorder
// candidates that retrieval already surfaced, so it must run second;
// generation consumes the reranker's narrowed, reordered candidates last.
func ragRerankerPlacementTest() testkit.Test {
	prompt := `A RAG pipeline has these three stages, listed here in no
particular order:
- retrieve: run a broad search (vector and/or keyword) over the index to
  produce an initial candidate set of chunks.
- rerank: use a more expensive, more accurate model (e.g. a cross-encoder)
  to reorder a candidate set by relevance and narrow it to the best few.
- generate: have the LLM produce the final answer using the selected
  chunks as context.

Give the correct pipeline order as a JSON array of the three stage names,
e.g. ["a","b","c"]. Respond with only the JSON array.`

	return testkit.Test{
		ID:          "rag-reranker-placement",
		Category:    "ai",
		Subcategory: "rag",
		Description: "Order the RAG pipeline stages: retrieve, rerank, generate.",
		Prompt:      prompt,
		Eval:        eval.JSONStringArrayEquals([]string{"retrieve", "rerank", "generate"}),
	}
}

// ragRetrievalFailureModeCorpus is the inline single-document corpus for
// ragRetrievalFailureModeTest, chosen so the query shares no exact term
// with the relevant document.
const ragRetrievalFailureModeCorpus = `Document 1: "Regular automobile maintenance, including oil changes and
brake inspections, prevents most roadside breakdowns."`

// ragRetrievalFailureModeTest: identify that a purely keyword/exact-term
// search fails to retrieve a relevant document when the query uses
// synonyms with no literal term overlap, while semantic (embedding-based)
// search would succeed.
//
// ground truth: the query "car repair tips" shares zero exact terms with
// Document 1's text ("automobile", "maintenance", "oil changes", "brake
// inspections") - "car" vs "automobile" and "repair" vs "maintenance" are
// synonym pairs, not shared tokens. A keyword/exact-term search over these
// tokens finds no match and fails to retrieve this relevant document;
// semantic (embedding-based) search, which captures the synonym
// relationship, would retrieve it.
func ragRetrievalFailureModeTest() testkit.Test {
	prompt := `Here is a single-document corpus:

` + ragRetrievalFailureModeCorpus + `

Query: "car repair tips"

Document 1 is genuinely relevant to this query, but one retrieval method
fails to retrieve it because the query and the document share no exact,
literal terms (only synonym pairs like "car"/"automobile" and
"repair"/"maintenance"). Which retrieval method is expected to fail here:
keyword-based (exact term matching, e.g. BM25) or semantic (embedding-
based)? Respond with only one word: keyword or semantic.`

	return testkit.Test{
		ID:          "rag-retrieval-failure-mode",
		Category:    "ai",
		Subcategory: "rag",
		Description: "Identify keyword (exact-term) search as the method that fails on a synonym-only query/document pair.",
		Prompt:      prompt,
		Eval:        eval.Equals("keyword"),
	}
}

// ragCitationGroundingTest: state the core RAG grounding requirement -
// answers must be traceable to retrieved context, and the system must
// decline (rather than guess from parametric knowledge) when the context
// does not contain the answer.
func ragCitationGroundingTest() testkit.Test {
	prompt := `State the core grounding requirement a RAG system's answer
generation must follow to avoid hallucination: what must every claim in
the answer be traceable back to, and what must the system do instead of
guessing when the retrieved context does not actually contain the
information needed to answer the question?`

	evaluator := eval.Mean(
		eval.ContainsAny("cite", "citation", "source", "quote", "quoted"),
		eval.ContainsAny("insufficient", "does not contain", "doesn't contain", "cannot answer", "can't answer", "no relevant", "unable to find", "not found in the context", "not present in the context"),
	)

	return testkit.Test{
		ID:          "rag-citation-grounding",
		Category:    "ai",
		Subcategory: "rag",
		Description: "State that answers must cite retrieved context and the system must decline when context lacks the answer, rather than guessing.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// ragContextSelectionVsStuffingTest: choose relevance-filtered ("selective")
// context assembly over cramming every chunk the budget allows
// ("stuffed"), given a stated fact that low-relevance chunks degrade
// answer quality.
//
// ground truth: the prompt states, as a given fact, that including
// low-relevance chunks measurably degrades answer quality even when
// budget remains. Under that stated fact, filling the entire available
// budget with the 20 ranked chunks ("stuffed") would knowingly include
// low-relevance chunks past the point where relevance drops off, so the
// correct approach is "selective": include only chunks above a relevance
// threshold, even when that uses less than the full budget.
func ragContextSelectionVsStuffingTest() testkit.Test {
	prompt := `A retriever ranks 20 chunks by relevance score for a query.
The context window budget has room for 15 of those 20 chunks. It is an
established fact for this system that including low-relevance chunks
measurably degrades answer quality, even when there is still budget room
left for them.

Only the top 6 of the 20 ranked chunks score above the system's relevance
threshold; the remaining 14 score below it.

Given the stated fact above, should you assemble context by including all
15 chunks the budget allows ("stuffed"), or by including only the 6
chunks that clear the relevance threshold, leaving the rest of the budget
unused ("selective")? Respond with only one word: stuffed or selective.`

	return testkit.Test{
		ID:          "rag-context-selection-vs-stuffing",
		Category:    "ai",
		Subcategory: "rag",
		Description: "Choose relevance-filtered ('selective') context assembly over budget-filling ('stuffed') given low-relevance chunks degrade quality.",
		Prompt:      prompt,
		Eval:        eval.Equals("selective"),
	}
}

// ragIndexStalenessTest: diagnose stale chunks coexisting with fresh ones
// after a naive re-embed pipeline, and pick the fix.
//
// ground truth: a pipeline that only inserts newly embedded chunks for an
// updated document, without deleting that document's previous chunks
// first, leaves both the old (stale) and new (fresh) chunks in the index
// simultaneously. Retrieval can then surface the stale version alongside
// or instead of the current one. The fix is to delete the document's old
// chunks before (or atomically with) inserting the new ones, not to
// change chunk overlap or top-k, which do not address stale chunks
// remaining in the index at all.
func ragIndexStalenessTest() testkit.Test {
	prompt := `A document is updated from v1 to v2. The ingestion pipeline
re-chunks and re-embeds only the new v2 text, then inserts those new
chunks into the index. It never removes the v1 chunks that were inserted
when the document was first indexed. After this update, queries about the
document sometimes return v1's outdated chunks.

What problem does this describe, and what is the correct fix? Respond
with only a JSON object:
{"problem":"stale-chunks-coexist"|"embedding-model-drift"|"query-cache-stale",
 "fix":"delete-old-chunks-before-reinsert"|"increase-chunk-overlap"|"raise-top-k"}`

	evaluator := eval.Mean(
		eval.JSONField("problem", "stale-chunks-coexist"),
		eval.JSONField("fix", "delete-old-chunks-before-reinsert"),
	)

	return testkit.Test{
		ID:          "rag-index-staleness",
		Category:    "ai",
		Subcategory: "rag",
		Description: "Diagnose stale v1 chunks coexisting with v2 chunks after a naive re-embed, and pick delete-before-reinsert as the fix.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// ragEvalMetricChoiceTest: map two scenarios to the RAG evaluation metric
// that captures each, using definitions given entirely inline so the
// answer follows from the prompt rather than memorized metric names.
//
// ground truth: scenario_a describes irrelevant chunks diluting the
// retrieved set (3 relevant out of 10 retrieved) - exactly the definition
// given for context_precision (fraction of retrieved chunks that are
// relevant). scenario_b describes claims in the generated answer that are
// not supported by the retrieved context (invented content) - exactly the
// definition given for faithfulness (fraction of generated claims
// supported by the retrieved context).
func ragEvalMetricChoiceTest() testkit.Test {
	prompt := `Three RAG evaluation metrics, defined here exactly:
- context_precision: of the chunks that were retrieved, what fraction are
  actually relevant to the query.
- context_recall: of the chunks that are truly relevant to the query, what
  fraction were retrieved.
- faithfulness: of the individual factual claims made in the generated
  answer, what fraction are directly supported by the retrieved context.

scenario_a: "Retrieval returned 10 chunks for a query; only 3 of those 10
were actually relevant to the query."

scenario_b: "The generated answer made 5 factual claims; 2 of those 5
claims were not supported by anything in the retrieved context - the
model invented them."

Using only the definitions given above, which single metric captures the
specific problem in each scenario? Respond with only a JSON object:
{"scenario_a":"context_precision"|"context_recall"|"faithfulness",
 "scenario_b":"context_precision"|"context_recall"|"faithfulness"}`

	evaluator := eval.Mean(
		eval.JSONField("scenario_a", "context_precision"),
		eval.JSONField("scenario_b", "faithfulness"),
	)

	return testkit.Test{
		ID:          "rag-eval-metric-choice",
		Category:    "ai",
		Subcategory: "rag",
		Description: "Map two RAG failure scenarios to context_precision and faithfulness using inline metric definitions.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// ragHallucinationMitigationOrderingTest: order 5 hallucination-mitigation
// pipeline steps.
//
// ground truth: grounding context must be retrieved before the model can
// be instructed to use it; the instruction to cite the context is part of
// the prompt assembled before generation; generation itself must run
// before its output can be verified; verification must run before
// unsupported claims can be flagged/removed, since flagging depends on
// verification's result.
func ragHallucinationMitigationOrderingTest() testkit.Test {
	prompt := `A RAG pipeline's hallucination-mitigation steps are listed
here, identified by id, in no particular order:
- step_retrieve: retrieve grounding context relevant to the query.
- step_instruct: instruct the model, in the prompt, to answer using only
  the retrieved context and to cite it.
- step_generate: generate the answer.
- step_verify: verify each claim in the generated answer against the
  retrieved context.
- step_flag: flag or remove any claim that verification could not support.

Give the correct order these steps must run in, as a JSON array of their
ids, e.g. ["step_a","step_b"]. Respond with only the JSON array.`

	return testkit.Test{
		ID:          "rag-hallucination-mitigation-ordering",
		Category:    "ai",
		Subcategory: "rag",
		Description: "Order 5 hallucination-mitigation pipeline steps: retrieve, instruct, generate, verify, flag.",
		Prompt:      prompt,
		Eval: eval.JSONStringArrayEquals([]string{
			"step_retrieve", "step_instruct", "step_generate", "step_verify", "step_flag",
		}),
	}
}

// ragMultihopDecompositionTest: decompose a multi-hop question into its
// ordered sub-question fragments, selecting only the genuinely needed
// fragments from a roster that includes one irrelevant distractor.
//
// ground truth: answering "what is the annual maintenance cost of the
// storage tier used by the database that backs the service which handles
// user authentication" requires, in strict dependency order: (1) which
// service handles user authentication (frag_q), (2) which database backs
// that service (frag_p, depends on frag_q's answer), (3) which storage
// tier that database uses (frag_s, depends on frag_p's answer), (4) that
// storage tier's annual maintenance cost (frag_r, depends on frag_s's
// answer). frag_x (team headcount) answers a question never asked and
// must be excluded.
func ragMultihopDecompositionTest() testkit.Test {
	prompt := `Multi-hop question: "What is the annual maintenance cost of
the storage tier used by the database that backs the service which
handles user authentication?"

Candidate sub-question fragments, identified by id, in no particular
order:
- frag_q: "Which service handles user authentication?"
- frag_p: "Which database backs that service?"
- frag_s: "Which storage tier does that database use?"
- frag_r: "What is the annual maintenance cost of that storage tier?"
- frag_x: "What is the total headcount of the team that owns that
  service?"

Exactly one of these fragments (frag_x) does not help answer the
multi-hop question and must be excluded. Give the remaining fragments, in
the order they must be answered (each depends on the previous answer), as
a JSON array of their ids. Respond with only the JSON array.`

	return testkit.Test{
		ID:          "rag-multihop-decomposition",
		Category:    "ai",
		Subcategory: "rag",
		Description: "Decompose a 4-hop question into its ordered sub-question fragments, excluding one irrelevant distractor.",
		Prompt:      prompt,
		Eval:        eval.JSONStringArrayEquals([]string{"frag_q", "frag_p", "frag_s", "frag_r"}),
	}
}

// ragPreAssemblyDedupTest: explain why deduplicating near-identical
// retrieved chunks before assembling context matters.
func ragPreAssemblyDedupTest() testkit.Test {
	prompt := `A query's top-8 retrieved chunks include 3 chunks that are
near-identical copies of the same FAQ answer, mirrored across 3 different
source pages, all of which scored highly enough to land in the top 8.

Explain why deduplicating these near-identical chunks before assembling
the final context (rather than passing all 3 copies straight through to
the LLM) matters: name what assembling all 3 copies wastes, and what
problem it risks introducing into the generated answer.`

	evaluator := eval.Mean(
		eval.ContainsAny("token budget", "context window", "context budget", "waste", "wastes", "wasted"),
		eval.ContainsAny("redundant", "redundancy", "duplicate", "duplication", "repetition", "repetitive", "diversity", "biased", "bias", "skew"),
	)

	return testkit.Test{
		ID:          "rag-preassembly-dedup",
		Category:    "ai",
		Subcategory: "rag",
		Description: "Explain why deduplicating near-identical retrieved chunks before context assembly matters (wasted budget, redundancy/bias risk).",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}
