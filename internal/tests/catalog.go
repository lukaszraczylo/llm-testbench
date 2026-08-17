package tests

import "github.com/lukaszraczylo/llm-testbench/internal/testkit"

// All builds a Registry containing the full 17-test catalog: 3
// programming/golang, 2 programming/python, 1 programming/typescript, 1
// programming/c, 2 operations/macos, 2 operations/linux, 1
// operations/kubernetes, 1 research/web, 1 research/whitepapers, 1
// research/codebase, and 2 agents (tool-routing, planning).
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
	return r
}
