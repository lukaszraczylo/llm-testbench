package tests

import (
	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

func registerPythonTests(r *testkit.Registry) {
	r.Register(pyLogTriageTest())
	r.Register(pyCosineTest())
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
