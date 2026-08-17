package tests

import (
	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

func registerAgentsTests(r *testkit.Registry) {
	r.Register(agentToolRoutingTest())
	r.Register(agentPlanOrderingTest())
}

// agentToolRoutingRoster is the inline tool roster for
// agentToolRoutingTest.
const agentToolRoutingRoster = `- search_web: Search the public web for current information not in your
  training data.
- read_file: Read the contents of a file from the local filesystem.
- run_shell: Execute an arbitrary shell command on the local machine.
- query_db: Run a read-only SQL query against the application's Postgres
  database.
- send_email: Send an email to a specified recipient.
- none: No tool is needed; answer directly from what you already know.`

// agentToolRoutingTest: map 4 mini-tasks to the correct tool from a 6-tool
// roster.
//
// ground truth: task1 is arithmetic answerable without any tool (none);
// task2 needs a row count from a database table (query_db); task3 asks for
// current information that cannot be in training data (search_web); task4
// asks to notify someone by email (send_email).
func agentToolRoutingTest() testkit.Test {
	prompt := `You have access to this roster of tools:

` + agentToolRoutingRoster + `

For each of these 4 tasks, decide which single tool (or "none") is the
correct one to use:

task1: "What is 15% of 240?"
task2: "How many rows are in the 'orders' table?"
task3: "What is today's top headline on a major news site?"
task4: "Notify the on-call engineer at oncall@example.com that the deploy
finished."

Respond with only a JSON object mapping each task id to the tool name:
{"task1":"...","task2":"...","task3":"...","task4":"..."}`

	evaluator := eval.Mean(
		eval.JSONField("task1", "none"),
		eval.JSONField("task2", "query_db"),
		eval.JSONField("task3", "search_web"),
		eval.JSONField("task4", "send_email"),
	)

	return testkit.Test{
		ID:          "agent-tool-routing",
		Category:    "agents",
		Subcategory: "tool-routing",
		Description: "Map 4 mini-tasks to the correct tool from a 6-tool roster.",
		Prompt:      prompt,
		MaxTokens:   300,
		Eval:        evaluator,
	}
}

// agentPlanOrderingWant is the unique valid step order implied by the
// dependency constraints in agentPlanOrderingTest's prompt.
var agentPlanOrderingWant = []string{"build", "test", "backup", "deploy", "verify", "rollback"}

// agentPlanOrderingTest: order deployment steps that have explicit
// pairwise dependencies.
//
// ground truth: the prompt states build must precede test (need a built
// artifact to test), backup must happen only after tests pass and only
// before deploy, deploy must follow backup, verify must follow deploy, and
// rollback is only meaningful after a deploy whose verify step failed. This
// pins a single valid total order: build, test, backup, deploy, verify,
// rollback.
func agentPlanOrderingTest() testkit.Test {
	prompt := `You are planning a deployment with these steps: build, test,
backup, deploy, verify, rollback.

Constraints:
- You must build the artifact before you can test it.
- You only take a backup of the current production state once tests have
  passed - there is no point backing up before you know the new build is
  viable.
- Deploy only happens after the backup completes.
- After deploy, you must verify the new deployment is healthy.
- Rollback (restoring the pre-deploy backup) only makes sense after a
  deploy has happened, and only if verify fails.

Give the single correct step order as a JSON array of step ids, e.g.
["build","test",...]. Include all 6 steps, listing rollback in its position
in the sequence even though it is conditional on verify failing.`

	return testkit.Test{
		ID:          "agent-plan-ordering",
		Category:    "agents",
		Subcategory: "planning",
		Description: "Order 6 deployment steps under explicit pairwise dependency constraints.",
		Prompt:      prompt,
		MaxTokens:   300,
		Eval:        eval.JSONStringArrayEquals(agentPlanOrderingWant),
	}
}
