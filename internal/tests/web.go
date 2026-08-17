package tests

import (
	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

func registerWebTests(r *testkit.Registry) {
	r.Register(webRobotsAICrawlersTest())
	r.Register(webHTMLProductExtractTest())
	r.Register(webHreflangReciprocalTest())
	r.Register(webURLParseComponentsTest())
	r.Register(webSitemapMaxURLsTest())
	r.Register(webHTTPStatusScenariosTest())
	r.Register(webDNSMXRecordTest())
	r.Register(webCanonicalVsRedirectTest())
	r.Register(webCORSMissingHeaderTest())
	r.Register(webSecurityHeadersAuditTest())
}

// webRobotsAICrawlersRobotsTxt is the inline robots.txt for
// webRobotsAICrawlersTest. Each listed user-agent has an explicit block, so
// the most specific matching block applies per bot (standard robots.txt
// precedence); the wildcard block only applies to bots with no dedicated
// block of their own.
const webRobotsAICrawlersRobotsTxt = `User-agent: GPTBot
Disallow: /

User-agent: ClaudeBot
Allow: /

User-agent: PerplexityBot
Disallow: /docs/

User-agent: Googlebot
Allow: /

User-agent: *
Disallow: /private/`

// webRobotsAICrawlersTest: determine which AI crawlers (not just any
// allowed bot) can fetch /docs/.
//
// ground truth: for path /docs/, GPTBot's dedicated block disallows /
// entirely (blocked), ClaudeBot's dedicated block allows / (can fetch),
// PerplexityBot's dedicated block disallows /docs/ specifically (blocked),
// and Googlebot's dedicated block allows / (can fetch) - but Googlebot is
// Google's general-purpose search-indexing crawler, not an AI crawler, so
// it does not belong in an "AI crawlers" answer even though it is
// technically allowed. The only AI crawler that can fetch /docs/ is
// ClaudeBot.
func webRobotsAICrawlersTest() testkit.Test {
	prompt := `Here is a robots.txt file:

` + webRobotsAICrawlersRobotsTxt + `

GPTBot (OpenAI), ClaudeBot (Anthropic), and PerplexityBot (Perplexity) are
AI crawlers used to fetch content for AI systems. Googlebot is Google's
general-purpose search-indexing crawler, not an AI crawler.

Which of the AI crawlers listed above are permitted, by this robots.txt, to
fetch a page at path /docs/? Respond with only a JSON array of crawler
names, e.g. ["ClaudeBot"]. Do not include Googlebot even if it is allowed,
since it is not an AI crawler.`

	return testkit.Test{
		ID:          "web-robots-ai-crawlers",
		Category:    "research",
		Subcategory: "web",
		Description: "Determine which AI crawlers (excluding a general search-engine decoy) may fetch a path per robots.txt rules.",
		Prompt:      prompt,
		MaxTokens:   300,
		Eval:        eval.JSONStringSet([]string{"ClaudeBot"}),
	}
}
