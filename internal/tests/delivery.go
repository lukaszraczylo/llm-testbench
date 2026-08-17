package tests

import "github.com/lukaszraczylo/llm-testbench/internal/testkit"

// registerDeliveryTests registers every delivery-category test. Each
// subcategory - git, containers, and release-engineering - has its own
// register function and source file (delivery_git.go,
// delivery_containers.go, delivery_release.go) to keep any one file from
// growing past a few hundred lines, mirroring the agents category's split
// (agents.go / agents_toolrouting.go / agents_planning.go /
// agents_delegation.go).
func registerDeliveryTests(r *testkit.Registry) {
	registerDeliveryGitTests(r)
	registerDeliveryContainersTests(r)
	registerDeliveryReleaseTests(r)
}
