package tests

import "github.com/lukaszraczylo/llm-testbench/internal/testkit"

// registerAgentsTests registers every agents-category test. Each
// subcategory - tool-routing, planning, and delegation - has its own
// register function and source file (agents_toolrouting.go,
// agents_planning.go, agents_delegation.go) to keep any one file from
// growing past a few hundred lines.
func registerAgentsTests(r *testkit.Registry) {
	registerAgentToolRoutingTests(r)
	registerAgentPlanningTests(r)
	registerAgentDelegationTests(r)
}
