package tests

import "github.com/lukaszraczylo/llm-testbench/internal/testkit"

// All builds a Registry containing the full test catalog, spanning the
// programming (golang, python, typescript, c), operations (macos, linux,
// kubernetes), research (web, whitepapers, codebase), and agents
// (tool-routing, planning) categories.
func All() *testkit.Registry {
	r := testkit.NewRegistry()
	registerGolangTests(r)
	registerPythonTests(r)
	registerTypeScriptTests(r)
	registerCTests(r)
	registerMacOSTests(r)
	registerLinuxTests(r)
	registerKubernetesTests(r)
	registerWebTests(r)
	registerWhitepapersTests(r)
	registerCodebaseTests(r)
	registerAgentsTests(r)
	registerDatabasesTests(r)
	registerSecurityTests(r)
	registerDeliveryTests(r)
	registerAITests(r)
	return r
}
