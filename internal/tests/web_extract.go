package tests

import (
	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

// webProductCardHTML is the inline HTML product-card snippet for
// webHTMLProductExtractTest.
const webProductCardHTML = `<div class="product-card" data-sku="WBH-2201">
  <h2 class="product-name">Wireless Mechanical Keyboard</h2>
  <span class="price" data-currency="USD">129.99</span>
  <span class="stock" data-available="true">In Stock</span>
</div>`

// webHTMLProductExtractTest: extract a product's name, price, and SKU
// from inline HTML into JSON.
//
// ground truth: name is the h2.product-name text; price is the
// span.price text as a number; sku is the data-sku attribute on the
// outer div - all read directly from the markup, no computation.
func webHTMLProductExtractTest() testkit.Test {
	prompt := `Here is an HTML product card:

` + "```html\n" + webProductCardHTML + "\n```" + `

Extract the product's name, price (as a number), and SKU into JSON.
Respond with only a JSON object: {"name":"...","price":<number>,"sku":"..."}`

	evaluator := eval.Mean(
		eval.JSONField("name", "Wireless Mechanical Keyboard"),
		eval.JSONField("price", 129.99),
		eval.JSONField("sku", "WBH-2201"),
	)

	return testkit.Test{
		ID:          "web-html-product-extract",
		Category:    "research",
		Subcategory: "web",
		Description: "Extract a product's name, price, and SKU from an inline HTML product card into JSON.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// webHreflangPagesSnippet lists the hreflang <link> tags of three pages
// for webHreflangReciprocalTest. /en/ links to both /es/ and /fr/, and
// /es/ links back to both, but /fr/ omits the reciprocal link back to
// /en/.
const webHreflangPagesSnippet = `Page https://example.com/en/ <head> contains:
<link rel="alternate" hreflang="en-us" href="https://example.com/en/" />
<link rel="alternate" hreflang="es" href="https://example.com/es/" />
<link rel="alternate" hreflang="fr" href="https://example.com/fr/" />

Page https://example.com/es/ <head> contains:
<link rel="alternate" hreflang="en-us" href="https://example.com/en/" />
<link rel="alternate" hreflang="es" href="https://example.com/es/" />
<link rel="alternate" hreflang="fr" href="https://example.com/fr/" />

Page https://example.com/fr/ <head> contains:
<link rel="alternate" hreflang="es" href="https://example.com/es/" />
<link rel="alternate" hreflang="fr" href="https://example.com/fr/" />`

// webHreflangReciprocalTest: find the one non-reciprocal hreflang link
// among three inline pages.
//
// ground truth: /en/ declares an hreflang link to /fr/, but /fr/'s own
// link set has no entry pointing back to https://example.com/en/, so the
// pair (/en/, /fr/) is not reciprocal; /fr/ is the page missing the link,
// and https://example.com/en/ is the target it is missing. Every other
// pair in the three pages is fully reciprocal.
func webHreflangReciprocalTest() testkit.Test {
	prompt := `hreflang annotations must be reciprocal: if page A links to page B
via hreflang, page B must also link back to page A. Here are the hreflang
link tags for three pages:

` + webHreflangPagesSnippet + `

Exactly one pair among these three pages is broken (not reciprocal).
Respond with only a JSON object:
{"page_missing_link":"<url of the page that is missing a link>","missing_target":"<url that page should link to but does not>"}`

	evaluator := eval.Mean(
		eval.JSONField("page_missing_link", "https://example.com/fr/"),
		eval.JSONField("missing_target", "https://example.com/en/"),
	)

	return testkit.Test{
		ID:          "web-hreflang-reciprocal",
		Category:    "research",
		Subcategory: "web",
		Description: "Find the one non-reciprocal hreflang link among three inline pages' link tag sets.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// webParseURL is the inline URL for webURLParseComponentsTest.
const webParseURL = "https://api.example.com:8443/v2/search?q=hello+world&sort=desc#results"

// webURLParseComponentsTest: parse an inline URL into its component parts
// as JSON.
//
// ground truth: computed directly from net/url.Parse in
// web_extract_test.go, not hand-derived - scheme "https", host (hostname,
// no port) "api.example.com", port "8443", path "/v2/search", raw query
// "q=hello+world&sort=desc", fragment "results".
func webURLParseComponentsTest() testkit.Test {
	prompt := `Parse this URL into its components:

` + webParseURL + `

Respond with only a JSON object:
{"scheme":"...","host":"...","port":<number>,"path":"...","query":"...","fragment":"..."}

"host" must be the hostname only (no port). "port" must be a number.
"query" must be the raw query string exactly as it appears after the "?",
not split into individual parameters.`

	evaluator := eval.Mean(
		eval.JSONField("scheme", "https"),
		eval.JSONField("host", "api.example.com"),
		eval.JSONField("port", 8443),
		eval.JSONField("path", "/v2/search"),
		eval.JSONField("query", "q=hello+world&sort=desc"),
		eval.JSONField("fragment", "results"),
	)

	return testkit.Test{
		ID:          "web-url-parse-components",
		Category:    "research",
		Subcategory: "web",
		Description: "Parse an inline URL into its scheme, host, port, path, query, and fragment components as JSON.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}
