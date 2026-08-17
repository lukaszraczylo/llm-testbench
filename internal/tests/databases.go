package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

// registerDatabasesTests registers every databases-category test. Each
// subcategory - postgres, redis, sql-tuning - has its own register function
// and source file (databases_postgres.go, databases_redis.go,
// databases_sqltuning.go) to keep any one file from growing past a few
// hundred lines. The orchestrator wires this into the catalog's All()
// separately; this package intentionally does not register itself in
// catalog.go so parallel category worktrees stay independently buildable.
func registerDatabasesTests(r *testkit.Registry) {
	registerDBPostgresTests(r)
	registerDBRedisTests(r)
	registerDBSQLTuningTests(r)
}

// dbExactAnswer returns an Evaluator awarding full credit when the
// response, trimmed of whitespace, at most one layer of surrounding quote
// characters (' , ", or `), and a single trailing sentence-ending period,
// equals want case-insensitively. Used across the databases category for
// prompts that force a single short forced-vocabulary answer (e.g. "yes"
// or "no", "keyset" or "offset"): it accepts every materially-correct form
// of that answer (bare, quoted, differently-cased, or with trailing
// punctuation) without loosening the match to accept a wrong answer.
func dbExactAnswer(want string) eval.Evaluator {
	return eval.EvaluatorFunc(func(_ context.Context, response string) eval.Score {
		got := strings.TrimSpace(response)
		got = strings.Trim(got, "\"'`")
		got = strings.TrimSuffix(strings.TrimSpace(got), ".")
		got = strings.TrimSpace(got)
		wantTrimmed := strings.TrimSpace(want)
		if strings.EqualFold(got, wantTrimmed) {
			return eval.Score{Value: 1, Detail: fmt.Sprintf("equals %q", wantTrimmed)}
		}
		return eval.Score{Value: 0, Detail: fmt.Sprintf("got %q, want %q", got, wantTrimmed)}
	})
}

// dbJSONArrayLength returns an Evaluator awarding full credit when the
// response's extracted JSON array (see eval.ExtractJSON) has exactly want
// elements, zero otherwise. Paired with per-index eval.JSONField checks so
// an answer with extra or missing elements (e.g. forgetting to apply a
// HAVING filter, so an extra group leaks through) cannot score full credit
// merely by getting the checked indices right.
func dbJSONArrayLength(want int) eval.Evaluator {
	return eval.EvaluatorFunc(func(_ context.Context, response string) eval.Score {
		raw, err := eval.ExtractJSON(response)
		if err != nil {
			return eval.Score{Value: 0, Detail: err.Error()}
		}
		var arr []json.RawMessage
		if err := json.Unmarshal([]byte(raw), &arr); err != nil {
			return eval.Score{Value: 0, Detail: fmt.Sprintf("invalid JSON array: %v", err)}
		}
		if len(arr) == want {
			return eval.Score{Value: 1, Detail: fmt.Sprintf("array length %d", len(arr))}
		}
		return eval.Score{Value: 0, Detail: fmt.Sprintf("array length %d, want %d", len(arr), want)}
	})
}
