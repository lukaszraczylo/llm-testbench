package tests

import (
	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

// registerUltraTests adds the hardest single-shot reasoning tasks - bit-level
// semantics, number theory, IEEE-754 edge cases, language rounding rules,
// algorithmic worst-case bounds, and distributed-systems trade-offs. Each
// answer is a value a machine can verify exactly. They land in existing
// subcategories, so the catalog shape is unchanged.
func registerUltraTests(r *testkit.Registry) {
	r.Register(ultraCInt8SignedTest())
	r.Register(ultraHardModexpTest())
	r.Register(ultraHardFPEqualityTest())
	r.Register(ultraPyBankersRoundTest())
	r.Register(ultraSecReDoSTest())
	r.Register(ultraScenCAPPartitionTest())
	r.Register(ultraAIPQBytesTest())
	r.Register(ultraSQLDeadlockCycleTest())
}

// ultraCInt8SignedTest: interpret a raw byte as a signed 8-bit two's
// complement value. 0b10110100 = 180 unsigned; MSB set, so 180 - 256 = -76.
func ultraCInt8SignedTest() testkit.Test {
	return testkit.Test{
		ID:          "ultra-c-int8-signed",
		Category:    "programming",
		Subcategory: "c",
		Description: "Interpret the byte 0b10110100 as a signed 8-bit two's complement integer: -76.",
		Prompt: "A single byte holds the bit pattern 10110100. Interpreted as a " +
			"signed 8-bit integer in two's complement, what decimal value is it? " +
			"Respond with only the integer.",
		Eval: eval.Numeric(eval.ExtractLastNumber[int], -76, 0),
	}
}

// ultraHardModexpTest: modular exponentiation 7^123 mod 13 = 5.
func ultraHardModexpTest() testkit.Test {
	return testkit.Test{
		ID:          "ultra-hard-modexp",
		Category:    "programming",
		Subcategory: "hard",
		Description: "Compute 7^123 mod 13 = 5 (modular exponentiation, e.g. via Fermat's little theorem).",
		Prompt:      "Compute 7 raised to the power 123, modulo 13. Respond with only the integer result.",
		Eval:        eval.Numeric(eval.ExtractLastNumber[int], 5, 0),
	}
}

// ultraHardFPEqualityTest: in IEEE-754 double precision, 0.1 + 0.2 != 0.3.
func ultraHardFPEqualityTest() testkit.Test {
	return testkit.Test{
		ID:          "ultra-hard-fp-equality",
		Category:    "programming",
		Subcategory: "hard",
		Description: "Recognize that 0.1 + 0.2 == 0.3 is false in IEEE-754 double precision.",
		Prompt: "In standard IEEE-754 double-precision floating point, does the " +
			"expression (0.1 + 0.2) == 0.3 evaluate to true? Respond with only a " +
			"JSON object: {\"equal\": true|false}",
		Eval: eval.JSONField("equal", false),
	}
}

// ultraPyBankersRoundTest: Python 3 round() uses banker's rounding (round
// half to even): round(2.5)=2, round(3.5)=4, sum = 6.
func ultraPyBankersRoundTest() testkit.Test {
	return testkit.Test{
		ID:          "ultra-py-bankers-round",
		Category:    "programming",
		Subcategory: "python",
		Description: "Apply Python 3 banker's rounding: round(2.5)+round(3.5) = 2+4 = 6.",
		Prompt: "In Python 3, what is the value of round(2.5) + round(3.5)? " +
			"Account for Python's rounding mode. Respond with only the integer.",
		Eval: eval.Numeric(eval.ExtractLastNumber[int], 6, 0),
	}
}

// ultraSecReDoSTest: the regex (a+)+$ has catastrophic backtracking on an
// input of many 'a's followed by a non-matching character.
func ultraSecReDoSTest() testkit.Test {
	return testkit.Test{
		ID:          "ultra-sec-redos",
		Category:    "security",
		Subcategory: "appsec",
		Description: "Identify (a+)+$ as vulnerable to catastrophic-backtracking ReDoS on a long non-matching input.",
		Prompt: "A backtracking regex engine evaluates the pattern (a+)+$ against " +
			"an input of 40 'a' characters followed by a single '!'. Is this " +
			"pattern vulnerable to catastrophic backtracking (ReDoS) on such input? " +
			"Respond with only a JSON object: {\"vulnerable\": true|false}",
		Eval: eval.JSONField("vulnerable", true),
	}
}

// ultraScenCAPPartitionTest: a system that stays strongly consistent during
// a network partition must sacrifice availability (a CP system).
func ultraScenCAPPartitionTest() testkit.Test {
	return testkit.Test{
		ID:          "ultra-scen-cap-partition",
		Category:    "operations",
		Subcategory: "scenario",
		Description: "Under a network partition, a system choosing strong consistency sacrifices availability (CP).",
		Prompt: "During a network partition, a distributed database is configured " +
			"to refuse any read or write it cannot confirm against a quorum, so " +
			"clients on the minority side get errors rather than stale data. In CAP " +
			"terms, which property is this system sacrificing during the partition? " +
			"Respond with only a JSON object: " +
			"{\"sacrificed\": \"consistency\"|\"availability\"|\"partition-tolerance\"}",
		Eval: eval.JSONField("sacrificed", "availability"),
	}
}

// ultraAIPQBytesTest: product quantization memory. With 16 subquantizers,
// each code being 8 bits (1 byte), a vector's PQ code is 16 bytes,
// independent of the original dimensionality.
func ultraAIPQBytesTest() testkit.Test {
	return testkit.Test{
		ID:          "ultra-ai-pq-bytes",
		Category:    "ai",
		Subcategory: "vector-search",
		Description: "Compute product-quantization code size: 16 subquantizers at 8 bits each = 16 bytes per vector.",
		Prompt: "A vector index uses product quantization with 16 subquantizers, " +
			"each encoding its sub-vector into an 8-bit code, on originally " +
			"768-dimensional float32 vectors. How many bytes does the PQ code for " +
			"one vector occupy? Respond with only the integer number of bytes.",
		Eval: eval.Numeric(eval.ExtractLastNumber[int], 16, 0),
	}
}

// ultraSQLDeadlockCycleTest: two transactions acquiring locks in opposite
// order deadlock; the classic fix is a consistent global lock ordering.
func ultraSQLDeadlockCycleTest() testkit.Test {
	return testkit.Test{
		ID:          "ultra-sql-deadlock-cycle",
		Category:    "databases",
		Subcategory: "sql-tuning",
		Description: "Diagnose an opposite-lock-order deadlock and prescribe a consistent global lock ordering.",
		Prompt: "Transaction T1 updates row A then row B; concurrently T2 updates " +
			"row B then row A. Both block and the database aborts one with a " +
			"deadlock error. What is the standard design fix that prevents this " +
			"class of deadlock without reducing concurrency to one transaction at a " +
			"time? Respond with only a JSON object: " +
			"{\"fix\": \"consistent-lock-order\"|\"longer-timeout\"|\"serializable-isolation\"|\"more-retries\"}",
		Eval: eval.JSONField("fix", "consistent-lock-order"),
	}
}
