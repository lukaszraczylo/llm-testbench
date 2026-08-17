package tests

import (
	"math"

	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

func registerPythonTests(r *testkit.Registry) {
	r.Register(pyLogTriageTest())
	r.Register(pyCosineTest())
	r.Register(pySoftmaxTest())
	r.Register(pyAsyncioGatherTraceTest())
	r.Register(pyMutableDefaultArgTest())
	r.Register(pyDictComprehensionTraceTest())
	r.Register(pyGeneratorExhaustionTraceTest())
	r.Register(pyPathlibRewriteTest())
	r.Register(pyJSONTransformTest())
	r.Register(pyRegexLogExtractionTest())
}

// pyLogTriageLog is the inline kubelet log excerpt for pyLogTriageTest.
// ground truth: counting by hand (also re-derived in python_test.go): 15
// lines total, 6 non-failure lines ("Started container ..." x4,
// "Readiness probe succeeded" x1, plus one more "Started container
// postgres"), 5 "OOMKilled" lines, 4 "ImagePullBackOff" lines.
const pyLogTriageLog = `2026-08-10T10:00:01Z pod=web-7f9c kubelet: Started container app
2026-08-10T10:00:05Z pod=web-7f9c kubelet: OOMKilled: memory limit exceeded
2026-08-10T10:00:07Z pod=api-2b3d kubelet: Back-off pulling image "registry.example.com/api:v2": ImagePullBackOff
2026-08-10T10:00:09Z pod=cache-1a2b kubelet: Started container redis
2026-08-10T10:00:11Z pod=web-9d1e kubelet: OOMKilled: memory limit exceeded
2026-08-10T10:00:13Z pod=worker-4f5g kubelet: Back-off pulling image "registry.example.com/worker:v1": ImagePullBackOff
2026-08-10T10:00:15Z pod=db-6h7i kubelet: Started container postgres
2026-08-10T10:00:17Z pod=web-3j8k kubelet: OOMKilled: memory limit exceeded
2026-08-10T10:00:19Z pod=api-9l0m kubelet: Back-off pulling image "registry.example.com/api:v3": ImagePullBackOff
2026-08-10T10:00:21Z pod=queue-2n3o kubelet: Started container consumer
2026-08-10T10:00:23Z pod=web-5p6q kubelet: Readiness probe succeeded
2026-08-10T10:00:25Z pod=worker-7r8s kubelet: OOMKilled: memory limit exceeded
2026-08-10T10:00:27Z pod=cache-9t0u kubelet: Back-off pulling image "registry.example.com/cache:v1": ImagePullBackOff
2026-08-10T10:00:29Z pod=db-1v2w kubelet: Started container postgres
2026-08-10T10:00:31Z pod=api-3x4y kubelet: OOMKilled: memory limit exceeded`

// pyLogTriageWant is the expected stdout of a correct triage script:
// counts sorted alphabetically by failure type, "type=count" per line.
// ground truth: 4 ImagePullBackOff lines, 5 OOMKilled lines (see
// pyLogTriageLog comment above); "I" < "O" alphabetically.
const pyLogTriageWant = "ImagePullBackOff=4\nOOMKilled=5"

func pyLogTriageTest() testkit.Test {
	prompt := `Here are 15 lines of kubelet log output for several pods in one namespace:

` + pyLogTriageLog + `

Write a self-contained python3 script (embed the log lines above as data in
your script; do not read from stdin or a file) that counts occurrences of
each failure type - OOMKilled and ImagePullBackOff - and prints one line
per failure type as "type=count", sorted alphabetically by type name. Do
not count non-failure lines (container starts, successful probes). Print
exactly those lines and nothing else.`

	return testkit.Test{
		ID:          "py-log-triage",
		Category:    "programming",
		Subcategory: "python",
		Description: "Write a python3 script that triages kubelet log lines into failure-type counts.",
		System:      terseCodeOnly,
		Prompt:      prompt,
		MaxTokens:   700,
		Eval:        eval.PyRun(eval.PassthroughHarness, pyLogTriageWant),
	}
}

// pyCosineVectorA and pyCosineVectorB are the two 8-dimensional vectors for
// pyCosineTest.
var (
	pyCosineVectorA = []float64{0.12, 0.45, -0.33, 0.78, 0.05, -0.61, 0.29, 0.14}
	pyCosineVectorB = []float64{0.22, 0.39, -0.28, 0.65, 0.11, -0.55, 0.31, 0.09}
)

// pyCosineWant is derived, not hardcoded: it calls the same
// cosineSimilarity helper that python_test.go independently re-verifies
// against math.Sqrt.
// ground truth: cosine(a,b) = dot(a,b) / (|a| * |b|), rounded to 4 decimal
// places as the prompt requests.
var pyCosineWant = round4dp(cosineSimilarity(pyCosineVectorA, pyCosineVectorB))

func pyCosineTest() testkit.Test {
	prompt := `Given two 8-dimensional vectors:

a = [0.12, 0.45, -0.33, 0.78, 0.05, -0.61, 0.29, 0.14]
b = [0.22, 0.39, -0.28, 0.65, 0.11, -0.55, 0.31, 0.09]

Compute their cosine similarity, rounded to 4 decimal places. Respond with
only the number, nothing else.`

	return testkit.Test{
		ID:          "py-cosine",
		Category:    "programming",
		Subcategory: "python",
		Description: "Compute the cosine similarity of two inline 8-dimensional vectors to 4 decimal places.",
		Prompt:      prompt,
		MaxTokens:   200,
		Eval:        eval.Numeric(eval.ExtractLastNumber[float64], pyCosineWant, 0.0005),
	}
}

// pySoftmaxVector is the inline 5-dimensional vector for pySoftmaxTest.
var pySoftmaxVector = []float64{2.0, 1.0, 0.1, -1.0, 0.5}

// softmaxFirst computes the softmax probability of index 0 of v:
// exp(v[0]) / sum(exp(v[i]) for all i). Used to derive pySoftmaxWant at
// catalog-registration time rather than hardcoding a pre-computed constant,
// mirroring pyCosineWant's use of cosineSimilarity.
func softmaxFirst(v []float64) float64 {
	var sum float64
	for _, x := range v {
		sum += math.Exp(x)
	}
	return math.Exp(v[0]) / sum
}

// pySoftmaxWant is derived, not hardcoded: python_test.go independently
// re-verifies it against math.Exp.
// ground truth: softmax(v)[0] = exp(v[0]) / sum(exp(v[i])), rounded to 4
// decimal places as the prompt requests.
var pySoftmaxWant = round4dp(softmaxFirst(pySoftmaxVector))

func pySoftmaxTest() testkit.Test {
	prompt := `Given the 5-dimensional vector:

v = [2.0, 1.0, 0.1, -1.0, 0.5]

Compute the softmax of v, and report only the probability assigned to the
first element (index 0), rounded to 4 decimal places. Respond with only the
number, nothing else.`

	return testkit.Test{
		ID:          "py-softmax",
		Category:    "programming",
		Subcategory: "python",
		Description: "Compute the softmax probability of one element of an inline 5-dimensional vector to 4 decimal places.",
		Prompt:      prompt,
		Eval:        eval.Numeric(eval.ExtractLastNumber[float64], pySoftmaxWant, 0.0005),
	}
}

// pyAsyncioGatherTraceCode is the inline snippet for
// pyAsyncioGatherTraceTest.
const pyAsyncioGatherTraceCode = `import asyncio


async def worker(name, delay):
    await asyncio.sleep(delay)
    print(name)


async def main():
    await asyncio.gather(worker("A", 0.02), worker("B", 0.01), worker("C", 0.03))


asyncio.run(main())`

// pyAsyncioGatherTraceTest: trace the print order of three coroutines
// started together via asyncio.gather with different sleep delays.
//
// ground truth: asyncio.gather starts all three coroutines concurrently on
// the single-threaded event loop; each immediately hits its
// asyncio.sleep(delay) and the loop resumes them in order of scheduled wake
// time (current loop time + delay), not source order. worker("B", 0.01) has
// the shortest delay, so it wakes and prints first, then worker("A", 0.02),
// then worker("C", 0.03). Because the event loop is single-threaded and
// wake times are computed from fixed delays far apart (10ms increments),
// this ordering has no OS-scheduling race and is deterministic - confirmed
// by running the exact snippet 5 times during authoring (always B, A, C)
// and independently by python_test.go's PyRun-based ground-truth test.
func pyAsyncioGatherTraceTest() testkit.Test {
	prompt := `Here is a Python program:

` + "```python" + `
` + pyAsyncioGatherTraceCode + `
` + "```" + `

In what order are the three names printed when this program runs? Respond
with only a JSON array of the three names in print order, for example
["X","Y","Z"]`

	return testkit.Test{
		ID:          "py-asyncio-gather-trace",
		Category:    "programming",
		Subcategory: "python",
		Description: "Trace the print order of three asyncio.gather coroutines with different sleep delays.",
		System:      terseCodeOnly,
		Prompt:      prompt,
		Eval:        eval.JSONStringArrayEquals([]string{"B", "A", "C"}),
	}
}

// pyMutableDefaultArgCode is the inline snippet for pyMutableDefaultArgTest.
const pyMutableDefaultArgCode = `def add_item(item, bucket=[]):
    bucket.append(item)
    return bucket


add_item(1)
add_item(2)
result = add_item(3)
print(result)`

// pyMutableDefaultArgTest: trace the classic mutable-default-argument bug.
//
// ground truth: a default argument value (here, the empty list literal) is
// evaluated once, at function-definition time, and the same list object is
// reused as the default on every call that omits bucket - it is not
// recreated per call. The three calls all omit bucket, so they share and
// mutate one underlying list: after add_item(1) it holds [1], after
// add_item(2) it holds [1, 2], and result = add_item(3) returns the same
// list now holding [1, 2, 3]. print(result) prints Python's list repr,
// "[1, 2, 3]". python_test.go re-verifies this via PyRun on the identical
// snippet.
func pyMutableDefaultArgTest() testkit.Test {
	prompt := `Here is a Python program:

` + "```python" + `
` + pyMutableDefaultArgCode + `
` + "```" + `

What does this program print when run? Respond with only the exact text
printed, nothing else.`

	return testkit.Test{
		ID:          "py-mutable-default-arg",
		Category:    "programming",
		Subcategory: "python",
		Description: "Trace the classic mutable-default-argument bug across three calls sharing one list.",
		Prompt:      prompt,
		// A7: eval.ExactToken so a fenced or quoted rendering of the exact
		// printed text still scores full credit.
		Eval: eval.ExactToken("[1, 2, 3]"),
	}
}

// pyDictComprehensionTraceCode is the inline snippet for
// pyDictComprehensionTraceTest.
const pyDictComprehensionTraceCode = `words = ["apple", "banana", "cherry", "date", "fig"]
lengths = {w: len(w) for w in words if len(w) > 3}
print(sorted(lengths.items()))`

// pyDictComprehensionTraceTest: trace the exact output of a filtered dict
// comprehension whose items are then sorted.
//
// ground truth: the comprehension keeps words with len > 3 - apple(5),
// banana(6), cherry(6), date(4) - and drops fig(3). sorted() on a dict's
// .items() sorts the (key, value) tuples by key first (Python tuple
// ordering compares element-by-element), giving alphabetical key order:
// apple, banana, cherry, date. print() renders that list of tuples with
// Python's repr, producing the exact text below. python_test.go
// re-verifies this via PyRun on the identical snippet.
func pyDictComprehensionTraceTest() testkit.Test {
	prompt := `Here is a Python program:

` + "```python" + `
` + pyDictComprehensionTraceCode + `
` + "```" + `

What does this program print when run? Respond with only the exact text
printed, nothing else.`

	return testkit.Test{
		ID:          "py-dict-comprehension-trace",
		Category:    "programming",
		Subcategory: "python",
		Description: "Trace the exact output of a length-filtered dict comprehension whose items are sorted and printed.",
		Prompt:      prompt,
		// A7: eval.ExactToken so a fenced or quoted rendering of the exact
		// printed text still scores full credit.
		Eval: eval.ExactToken("[('apple', 5), ('banana', 6), ('cherry', 6), ('date', 4)]"),
	}
}

// pyGeneratorExhaustionTraceCode is the inline snippet for
// pyGeneratorExhaustionTraceTest.
const pyGeneratorExhaustionTraceCode = `def gen():
    for i in range(3):
        yield i


g = gen()
first_pass = list(g)
second_pass = list(g)
print(first_pass, second_pass)`

// pyGeneratorExhaustionTraceTest: trace the exact output of iterating an
// already-exhausted generator a second time.
//
// ground truth: a Python generator object is a single-pass iterator - once
// list(g) has driven it to completion (StopIteration), the same generator
// object has no more items to yield. first_pass = list(g) consumes all
// three values, giving [0, 1, 2]; second_pass = list(g) on the same,
// now-exhausted g immediately raises StopIteration inside list(), which
// list() treats as "no items", giving []. print(a, b) separates its
// arguments with a single space, producing the exact text below.
// python_test.go re-verifies this via PyRun on the identical snippet.
func pyGeneratorExhaustionTraceTest() testkit.Test {
	prompt := `Here is a Python program:

` + "```python" + `
` + pyGeneratorExhaustionTraceCode + `
` + "```" + `

What does this program print when run? Respond with only the exact text
printed, nothing else.`

	return testkit.Test{
		ID:          "py-generator-exhaustion-trace",
		Category:    "programming",
		Subcategory: "python",
		Description: "Trace the exact output of iterating a generator a second time after it is already exhausted.",
		Prompt:      prompt,
		// A7: eval.ExactToken so a fenced or quoted rendering of the exact
		// printed text still scores full credit.
		Eval: eval.ExactToken("[0, 1, 2] []"),
	}
}

// pyOsPathCode is the inline os.path-based function pyPathlibRewriteTest
// asks the model to rewrite using pathlib.
const pyOsPathCode = `import os


def find_configs(base_dir):
    results = []
    for root, dirs, files in os.walk(base_dir):
        for f in files:
            if f.endswith(".yaml"):
                results.append(os.path.join(root, f))
    return results`

// pyPathlibRewriteTest: rewrite an os.path/os.walk function using pathlib
// idioms with equivalent behavior.
func pyPathlibRewriteTest() testkit.Test {
	prompt := `Here is a Python function that recursively finds all ".yaml" files
under base_dir and returns their full paths:

` + "```python" + `
` + pyOsPathCode + `
` + "```" + `

Rewrite this function using pathlib idioms instead of os.path/os.walk,
keeping the same behavior: recursively find every ".yaml" file under
base_dir and return their full paths. Respond with only the rewritten
function.`

	// A16: Path.rglob("*.yaml") and Path.glob("**/*.yaml") are behaviorally
	// equivalent recursive searches ("**/" is glob's own recursive
	// wildcard), so the recursive-method check accepts either instead of
	// only the literal substring "rglob"; the "*.yaml" pattern is checked
	// separately from which method carries it.
	evaluator := eval.All(
		eval.W(eval.ContainsAny("from pathlib import Path", "import pathlib"), 1),
		eval.W(eval.ContainsAny("rglob", `glob("**/`, "glob('**/"), 1),
		eval.W(eval.ContainsAll("*.yaml"), 1),
	)

	return testkit.Test{
		ID:          "py-pathlib-rewrite",
		Category:    "programming",
		Subcategory: "python",
		Description: "Rewrite an os.path/os.walk recursive file finder using pathlib idioms.",
		System:      terseCodeOnly,
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// pyJSONTransformRecords is the inline dataset for pyJSONTransformTest.
// ground truth: totals by dept, hand-summed (also re-derived in
// python_test.go by scanning the same literal data): eng = 95000 + 105000 +
// 88000 = 288000; ops = 72000; sales = 60000 + 58000 = 118000.
const pyJSONTransformRecords = `records = [
    {"dept": "eng", "salary": 95000},
    {"dept": "sales", "salary": 60000},
    {"dept": "eng", "salary": 105000},
    {"dept": "ops", "salary": 72000},
    {"dept": "sales", "salary": 58000},
    {"dept": "eng", "salary": 88000},
]`

// pyJSONTransformWant is the expected stdout of a correct transform script:
// total salary per department, sorted alphabetically by department, as
// "dept=total" per line.
const pyJSONTransformWant = "eng=288000\nops=72000\nsales=118000"

func pyJSONTransformTest() testkit.Test {
	prompt := `Here is a Python list of employee records:

` + "```python" + `
` + pyJSONTransformRecords + `
` + "```" + `

Write a self-contained python3 script (embed the records above as data in
your script; do not read from stdin or a file) that sums salary per dept
and prints one line per department as "dept=total", sorted alphabetically by
department name. Print exactly those lines and nothing else.`

	return testkit.Test{
		ID:          "py-json-transform",
		Category:    "programming",
		Subcategory: "python",
		Description: "Write a python3 script that sums an inline list of salary records by department.",
		System:      terseCodeOnly,
		Prompt:      prompt,
		Eval:        eval.PyRun(eval.PassthroughHarness, pyJSONTransformWant),
	}
}

// pyRegexLogLines is the inline nginx-style access-log fixture for
// pyRegexLogExtractionTest.
// ground truth: HTTP status codes by line, in order: 200, 404, 200, 500,
// 200, 404, 500 - giving 200=3, 404=2, 500=2 (also re-derived in
// python_test.go by scanning the same literal data with Go's regexp).
const pyRegexLogLines = `log_lines = [
    '203.0.113.5 - - [10/Aug/2026:10:00:01 +0000] "GET /api/health HTTP/1.1" 200 512',
    '203.0.113.7 - - [10/Aug/2026:10:00:02 +0000] "GET /api/users HTTP/1.1" 404 128',
    '203.0.113.5 - - [10/Aug/2026:10:00:03 +0000] "POST /api/login HTTP/1.1" 200 256',
    '203.0.113.9 - - [10/Aug/2026:10:00:04 +0000] "GET /api/orders HTTP/1.1" 500 96',
    '203.0.113.5 - - [10/Aug/2026:10:00:05 +0000] "GET /api/health HTTP/1.1" 200 512',
    '203.0.113.7 - - [10/Aug/2026:10:00:06 +0000] "GET /api/missing HTTP/1.1" 404 128',
    '203.0.113.9 - - [10/Aug/2026:10:00:07 +0000] "GET /api/orders HTTP/1.1" 500 96',
]`

// pyRegexLogExtractionWant is the expected stdout: counts per HTTP status
// code, sorted ascending by the numeric code, as "code=count" per line.
const pyRegexLogExtractionWant = "200=3\n404=2\n500=2"

func pyRegexLogExtractionTest() testkit.Test {
	prompt := `Here are 7 lines of nginx-style access log output:

` + "```python" + `
` + pyRegexLogLines + `
` + "```" + `

Write a self-contained python3 script (embed the log_lines list above as
data in your script; do not read from stdin or a file) that uses the re
module to extract the 3-digit HTTP status code from each line (the number
that appears right after the quoted request, e.g. right after
"GET ... HTTP/1.1"), counts occurrences of each status code, and prints one
line per status code as "code=count", sorted ascending by the numeric code.
Print exactly those lines and nothing else.`

	return testkit.Test{
		ID:          "py-regex-log-extraction",
		Category:    "programming",
		Subcategory: "python",
		Description: "Write a python3 script that uses re to extract and count HTTP status codes from an inline access log.",
		System:      terseCodeOnly,
		Prompt:      prompt,
		Eval:        eval.PyRun(eval.PassthroughHarness, pyRegexLogExtractionWant),
	}
}
