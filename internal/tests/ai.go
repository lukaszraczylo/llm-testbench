package tests

import "github.com/lukaszraczylo/llm-testbench/internal/testkit"

// registerAITests registers every ai-category test. Each subcategory -
// vector-search, llm-integration, and rag - has its own register function
// and source file (ai_vectorsearch.go, ai_llmintegration.go, ai_rag.go) to
// keep any one file from growing past a few hundred lines, mirroring
// agents.go's split across its three subcategories.
func registerAITests(r *testkit.Registry) {
	registerAIVectorSearchTests(r)
	registerAILLMIntegrationTests(r)
	registerAIRAGTests(r)
}
