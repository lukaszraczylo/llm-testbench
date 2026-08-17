package tests

import (
	"context"
	"regexp"

	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

// webSitemapMaxURLsTest: state the sitemaps.org protocol's maximum URL
// count per sitemap file.
//
// ground truth: the sitemaps.org protocol caps a single sitemap file at
// 50,000 URLs (and 50MB uncompressed); a file exceeding that must be
// split across multiple sitemap files listed in a sitemap index.
func webSitemapMaxURLsTest() testkit.Test {
	prompt := `A site's sitemap.xml file lists 52,000 <url> entries inside a single
<urlset>. Per the sitemaps.org protocol, what is the maximum number of
URLs permitted in a single sitemap file (the limit this file violates)?
Respond with only the number.`

	return testkit.Test{
		ID:          "web-sitemap-max-urls",
		Category:    "research",
		Subcategory: "web",
		Description: "State the sitemaps.org protocol's maximum URL-per-file limit, applied to a scenario that exceeds it.",
		Prompt:      prompt,
		Eval:        eval.Numeric(eval.ExtractLastNumber[int], 50000, 0),
	}
}

// webHTTPStatusScenarios lists four inline request-handling scenarios for
// webHTTPStatusScenariosTest.
const webHTTPStatusScenarios = `scenario1: "The client sent a request body that is not valid JSON."
scenario2: "The client did not send any authentication credentials, and
the requested resource requires a logged-in user."
scenario3: "The client is authenticated, but their account does not have
permission to access the requested resource."
scenario4: "The resource has permanently moved to a new URL; the original
request was a GET, and the server wants clients (and search engines) to
use the new URL from now on."`

// webHTTPStatusScenariosTest: map 4 inline HTTP scenarios to their correct
// status codes.
//
// ground truth: malformed request body -> 400 Bad Request; missing
// credentials on a resource that requires them -> 401 Unauthorized;
// authenticated but lacking permission -> 403 Forbidden; permanent
// relocation of a GET request -> 301 Moved Permanently (B8: the scenario
// states the original request is a GET, so 308 Permanent Redirect - which
// exists specifically to guarantee an unchanged method and body on
// replay, relevant for POST/PUT - is not equally defensible here; 301 is
// the unique correct answer).
func webHTTPStatusScenariosTest() testkit.Test {
	prompt := `For each of these 4 request-handling scenarios, give the single most
correct HTTP status code:

` + webHTTPStatusScenarios + `

Respond with only a JSON object mapping each scenario id to its status
code as a number: {"scenario1":<number>,"scenario2":<number>,"scenario3":<number>,"scenario4":<number>}`

	evaluator := eval.Mean(
		eval.JSONField("scenario1", 400),
		eval.JSONField("scenario2", 401),
		eval.JSONField("scenario3", 403),
		eval.JSONField("scenario4", 301),
	)

	return testkit.Test{
		ID:          "web-http-status-scenarios",
		Category:    "research",
		Subcategory: "web",
		Description: "Map 4 inline request-handling scenarios to their correct HTTP status codes.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// webDNSMXRecordTrailingWordPattern strips an optional trailing
// "record"/"records" word (with or without a final period) that the
// prompt's own "the record type" phrasing invites a model to echo back,
// e.g. "MX record" or "MX records.", before the exact-token comparison.
var webDNSMXRecordTrailingWordPattern = regexp.MustCompile(`(?i)\s+records?\.?\s*$`)

// webDNSMXRecordEval wraps eval.ExactToken(want) to also accept a response
// carrying that optional trailing "record"/"records" word (B4).
func webDNSMXRecordEval(want string) eval.Evaluator {
	exact := eval.ExactToken(want)
	return eval.EvaluatorFunc(func(ctx context.Context, response string) eval.Score {
		stripped := webDNSMXRecordTrailingWordPattern.ReplaceAllString(response, "")
		return exact.Evaluate(ctx, stripped)
	})
}

// webDNSMXRecordTest: name the DNS record type used to publish a domain's
// prioritized mail servers.
//
// ground truth: mail-routing priority is published exclusively via MX
// (Mail Exchange) records; no other standard record type carries a
// priority value for mail delivery. B4: the prompt's own example list used
// to include "MX" itself, which the prior substring evaluator would then
// score correct even for a response that only restated the option list
// without ever answering the question; MX is no longer in the example
// list, and the evaluator now anchors to the whole answer instead of
// matching a substring anywhere in the response.
func webDNSMXRecordTest() testkit.Test {
	prompt := `example.com wants mail delivered to mailA.example.com as its primary
mail server (priority 10), falling back to mailB.example.com (priority
20) only if the primary is unreachable. Which single DNS record type
publishes this prioritized mail-routing configuration? Respond with only
the record type (e.g. A, AAAA, CNAME, TXT, NS).`

	return testkit.Test{
		ID:          "web-dns-mx-record",
		Category:    "research",
		Subcategory: "web",
		Description: "Name the DNS record type used to publish a domain's prioritized mail servers.",
		Prompt:      prompt,
		Eval:        webDNSMXRecordEval("MX"),
	}
}

// webCanonicalVsRedirectTest: choose between a canonical tag and a 301
// redirect for a duplicate-content scenario where the duplicate URLs must
// stay independently reachable.
//
// ground truth: a 301 redirect would break the tracking links (they would
// stop resolving to their own URL), so the correct fix is a canonical
// tag: it consolidates ranking signals onto the preferred URL while
// leaving every tracked variant independently reachable. B3: the prompt
// itself quotes both options ("canonical" or "redirect"), so scored with
// eval.ExactToken - the prior anchored regex required the bare word and
// scored a quoted or bolded correct answer 0.
func webCanonicalVsRedirectTest() testkit.Test {
	prompt := `An e-commerce site serves identical product content at these three
URLs, which must all keep working since they are used as tracking links
in email and social campaigns:

https://example.com/product?id=123
https://example.com/product?id=123&ref=email
https://example.com/product?id=123&ref=social

Search engines are treating these as duplicate content and splitting
ranking signal between them. You need to consolidate that ranking signal
onto https://example.com/product?id=123, while keeping all three URLs
independently reachable for the tracking links to keep working. Should
you use a canonical tag or a 301 redirect? Respond with only one word:
"canonical" or "redirect".`

	return testkit.Test{
		ID:          "web-canonical-vs-redirect",
		Category:    "research",
		Subcategory: "web",
		Description: "Choose canonical tag over 301 redirect for a duplicate-content scenario requiring all URL variants to stay reachable.",
		Prompt:      prompt,
		Eval:        eval.ExactToken("canonical"),
	}
}
