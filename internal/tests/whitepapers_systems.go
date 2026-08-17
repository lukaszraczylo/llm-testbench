package tests

import (
	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

// paperLSMExcerpt is a ~152-word, technically accurate original
// description of LSM-tree leveled compaction and write amplification
// written for this test, pinning a concrete 4-level rewrite path.
const paperLSMExcerpt = `An LSM-tree buffers writes in memory and periodically flushes them as
immutable sorted files, then merges those files together in the background
through a process called compaction, so reads do not have to check an
unbounded number of files. In leveled compaction, each level on disk is
merged into the next level down, and every byte that a compaction step
touches gets rewritten to a new file even though its value did not change.
A storage engine described in this report uses four levels, L0 through L3.
A byte first flushed into L0 is compacted into L1, then later compacted
from L1 into L2, and finally from L2 into L3 before it reaches its
long-term resting level, meaning it is rewritten three separate times after
its original flush write. Write amplification here is defined as total
bytes written to disk, including every rewrite, divided by the bytes of
user data originally written.`

// paperLSMWriteAmpWant is derived by calling wpLSMWriteAmplification, not
// hardcoded.
//
// ground truth: the excerpt states a byte is rewritten 3 times (L0->L1,
// L1->L2, L2->L3) after its original flush write, so total disk writes per
// user byte = 1 (original) + 3 (rewrites) = 4.
// whitepapers_systems_test.go independently recomputes this by counting
// the rewrite path described in the excerpt text.
var paperLSMWriteAmpWant = wpLSMWriteAmplification(3)

// paperLSMWriteAmplificationTest: derive an LSM-tree's write amplification
// factor from an inline excerpt describing a byte's full compaction path.
func paperLSMWriteAmplificationTest() testkit.Test {
	prompt := `Read this excerpt about LSM-tree compaction:

` + paperLSMExcerpt + `

Using the byte's full path through this system (an L0 flush, then
compaction into L1, then L2, then L3) and the write-amplification
definition given above, what is the write amplification factor for that
byte? Respond with only the number.`

	return testkit.Test{
		ID:          "paper-lsm-write-amplification",
		Category:    "research",
		Subcategory: "whitepapers",
		Description: "Derive an LSM-tree's write amplification factor from an inline excerpt describing a byte's full leveled-compaction path.",
		Prompt:      prompt,
		Eval:        eval.Numeric(eval.ExtractLastNumber[int], paperLSMWriteAmpWant, 0),
	}
}

// paperRaftExcerpt is a ~155-word, technically accurate original
// description of Raft's majority-quorum requirement written for this
// test, pinning a 5-node cluster.
const paperRaftExcerpt = `Raft is a consensus protocol that keeps a replicated log consistent across
a cluster of nodes by electing a single leader for each term and having
that leader replicate every log entry to the other nodes. A node starts an
election after its randomized election timeout, typically drawn from the
150 to 300 millisecond range in the original paper, expires without
hearing from a leader. A candidate becomes leader only once it collects
votes from a strict majority of all nodes in the cluster, not merely a
majority of the nodes currently reachable, and the same majority rule
governs when a log entry counts as committed. This report deployed Raft
across a cluster of N equal to 5 nodes, so both leader election and log
commitment require votes or acknowledgments from a majority quorum sized
relative to that total membership of 5, not to however many nodes happen to
be healthy at a given moment.`

// paperRaftQuorumWant and paperRaftFailuresWant are derived by calling
// wpRaftQuorumSize and wpRaftMaxTolerableFailures, not hardcoded.
//
// ground truth: quorum = floor(5/2)+1 = 3; a 5-node cluster can lose at
// most 5-3 = 2 nodes and still reach that quorum.
// whitepapers_systems_test.go independently recomputes both for several N.
var (
	paperRaftQuorumWant   = wpRaftQuorumSize(5)
	paperRaftFailuresWant = wpRaftMaxTolerableFailures(5)
)

// paperRaftQuorumFailuresTest: derive a Raft cluster's majority quorum
// size and maximum tolerable simultaneous failures from an inline excerpt
// and a stated cluster size.
func paperRaftQuorumFailuresTest() testkit.Test {
	prompt := `Read this excerpt about the Raft consensus protocol:

` + paperRaftExcerpt + `

For this N=5 node cluster:
1. What is the minimum number of votes/acknowledgments needed for a
   majority quorum?
2. How many simultaneous node failures can the cluster sustain and still
   reach that quorum?

Respond with only a JSON object: {"quorum_size":<number>,"max_tolerable_failures":<number>}`

	evaluator := eval.Mean(
		eval.JSONField("quorum_size", paperRaftQuorumWant),
		eval.JSONField("max_tolerable_failures", paperRaftFailuresWant),
	)

	return testkit.Test{
		ID:          "paper-raft-quorum-failures",
		Category:    "research",
		Subcategory: "whitepapers",
		Description: "Derive a 5-node Raft cluster's majority quorum size and maximum tolerable simultaneous failures from an inline excerpt.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// paperBTreeExcerpt is a ~146-word, technically accurate original
// description of a worst-case (minimum-fanout) B-tree capacity model
// written for this test, pinning order m and a target key count.
const paperBTreeExcerpt = `A B-tree of order m allows every internal node between ceil(m/2) and m
children, keeping the tree shallow and well balanced even as it grows,
which bounds the number of disk reads a lookup needs to at most one per
level. For capacity planning this report models the worst case: a tree
where every internal node has the minimum allowed fanout, f equals
ceil(m/2), rather than the maximum. With that minimum fanout, and counting
the root itself as branching level h equal to zero, a tree with h
branching levels can index at most f to the power of (h+1) leaf key slots,
since each additional level below the root multiplies the reachable leaf
count by f. This benchmark used order m equal to 200, giving a minimum
fanout f equal to 100, and needed to index at least 5,000,000 keys without
exceeding that capacity bound.`

// paperBTreeHeightWant is derived by calling wpBTreeMinBranchingLevels,
// not hardcoded.
//
// ground truth: smallest h with 100^(h+1) >= 5,000,000: h=0 -> 100;
// h=1 -> 10,000; h=2 -> 1,000,000 (still short); h=3 -> 100,000,000
// (first to clear 5,000,000). whitepapers_systems_test.go independently
// recomputes this with a plain loop over math.Pow.
var paperBTreeHeightWant = wpBTreeMinBranchingLevels(100, 5000000)

// paperBTreeHeightLevelsTest: derive the minimum number of branching
// levels a worst-case B-tree needs to index a target key count, from an
// inline excerpt's stated capacity formula.
func paperBTreeHeightLevelsTest() testkit.Test {
	prompt := `Read this excerpt about B-tree capacity planning:

` + paperBTreeExcerpt + `

Using the formula given above (f^(h+1) >= n keys, root counted as h=0),
what is the minimum number of branching levels h needed so this tree can
index at least 5,000,000 keys with f=100? Respond with only the number.`

	return testkit.Test{
		ID:          "paper-btree-height-levels",
		Category:    "research",
		Subcategory: "whitepapers",
		Description: "Derive the minimum branching-level count a worst-case-fanout B-tree needs to index a target key count.",
		Prompt:      prompt,
		Eval:        eval.Numeric(eval.ExtractLastNumber[int], paperBTreeHeightWant, 0),
	}
}

// paperCAPExcerpt is a ~135-word, technically accurate original
// description of the CAP theorem written for this test, describing a
// scenario that unambiguously chooses availability over consistency.
const paperCAPExcerpt = `The CAP theorem states that a distributed data store can provide at most
two of three guarantees during a network partition: consistency, meaning
every read sees the most recent write or an error; availability, meaning
every request receives a non-error response; and partition tolerance,
meaning the system keeps operating despite dropped or delayed messages
between nodes. Because network partitions are unavoidable in any real
distributed deployment, partition tolerance is treated as a fixed
requirement rather than a design choice, leaving consistency and
availability as the actual trade-off a system architect makes. A key-value
store described in this report was explicitly designed so that, during a
partition between two data centers, both sides keep accepting writes
independently rather than refusing requests, accepting that the two sides
may briefly disagree and need reconciliation once the partition heals.`

// paperCAPAvailabilityPattern anchors on the trimmed response being
// exactly the single word "availability" (case-insensitive, with an
// optional trailing period), accepting every materially-correct form of
// the forced one-word answer without also matching a response that names
// the wrong guarantee.
const paperCAPAvailabilityPattern = `(?i)^\s*availability\.?\s*$`

// paperCAPAvailabilityChoiceTest: identify, from an inline CAP-theorem
// excerpt and a described design decision, which guarantee (consistency or
// availability) the system prioritizes during a partition.
func paperCAPAvailabilityChoiceTest() testkit.Test {
	prompt := `Read this excerpt about the CAP theorem:

` + paperCAPExcerpt + `

Given this design decision - both data centers keep accepting writes
during the partition rather than refusing requests - which of the two
trade-off guarantees does this system prioritize during a partition, per
the CAP theorem? Respond with only one word: "Consistency" or
"Availability".`

	return testkit.Test{
		ID:          "paper-cap-availability-choice",
		Category:    "research",
		Subcategory: "whitepapers",
		Description: "Identify which CAP-theorem guarantee a described partition-handling design decision prioritizes.",
		Prompt:      prompt,
		Eval:        eval.Regex(paperCAPAvailabilityPattern),
	}
}

// paperAttentionExcerpt is a ~158-word, technically accurate original
// description of scaled dot-product attention's sqrt(d_k) scaling written
// for this test, pinning a concrete per-head dimension.
const paperAttentionExcerpt = `Scaled dot-product attention computes a weighted average of value vectors,
where the weight given to each value comes from a softmax over the dot
products between one query vector and every key vector. When query and key
entries are drawn independently with roughly unit variance, the variance of
their dot product grows in proportion to the dimensionality d_k of those
vectors, so without correction, larger d_k pushes the raw dot products to
extreme magnitudes. Feeding those extreme values into softmax drives most
of the output probability mass onto one entry and squashes the gradient
almost to zero everywhere else, stalling training. To counteract this, the
dot products are divided by the square root of d_k before the softmax,
which rescales their variance back down to roughly one regardless of
dimensionality. A benchmark model in this report used d_k equal to 64 per
attention head, so this scaling divided every raw dot product by the square
root of 64.`

// paperAttentionDK and paperAttentionScaleFactorWant pin the excerpt's
// stated d_k and its square root.
//
// ground truth: d_k = 64 (stated verbatim in the excerpt); scale factor =
// sqrt(64) = 8. whitepapers_systems_test.go independently recomputes the
// scale factor with math.Sqrt.
const (
	paperAttentionDK              = 64
	paperAttentionScaleFactorWant = 8
)

// paperAttentionScaleFactorTest: extract d_k and derive the sqrt(d_k)
// attention scale factor from an inline excerpt.
func paperAttentionScaleFactorTest() testkit.Test {
	prompt := `Read this excerpt about scaled dot-product attention:

` + paperAttentionExcerpt + `

For the benchmark model described:
1. What is d_k?
2. By what number are the raw dot products divided before the softmax
   (the scale factor)?

Respond with only a JSON object: {"d_k":<number>,"scale_factor":<number>}`

	evaluator := eval.Mean(
		eval.JSONField("d_k", paperAttentionDK),
		eval.JSONField("scale_factor", paperAttentionScaleFactorWant),
	)

	return testkit.Test{
		ID:          "paper-attention-scale-factor",
		Category:    "research",
		Subcategory: "whitepapers",
		Description: "Extract d_k and derive the sqrt(d_k) attention scale factor from an inline excerpt.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}
