// Package-level hardening tests for the five subcategories that measured
// 100% perfect-score across all three baseline models (see the health
// audit over the 2026-08-18 3-model run): agents/delegation,
// databases/postgres, research/web, security/appsec, security/secrets.
// Every test there was saturated - zero discrimination signal. These ten
// tests are trap-shaped on purpose: each embeds a near-miss answer that
// plausible-but-wrong reasoning produces, and every expected value carries
// a derivation comment.
package tests

import (
	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

// registerSaturationTests adds the hardening tests to the registry. They
// land in existing subcategories, so the per-category register functions
// stay untouched; the subcategory's test count only grows.
func registerSaturationTests(r *testkit.Registry) {
	r.Register(satDelegCapacityTrapTest())
	r.Register(satDelegNearMissRosterTest())
	r.Register(satPGCountStarTrapTest())
	r.Register(satPGTimeoutTrapTest())
	r.Register(satWebRobotsUAScopeTest())
	r.Register(satWebHreflangSelfTest())
	r.Register(satAppsecORMTrapTest())
	r.Register(satAppsecPOSTTrapTest())
	r.Register(satSecretsRotationMathTest())
	r.Register(satSecretsErrorPathTest())
}

// satDelegCapacityTrapTest: the roster's specialist for the job exists but
// is unavailable. Tests whether the model force-fits the task to a
// mismatched agent, over-coordinates with the orchestrator, or correctly
// keeps it in the main thread.
func satDelegCapacityTrapTest() testkit.Test {
	prompt := `You are coordinating agents. Available specialist roster:

` + delegRoster + `

Situation: the CI pipeline for the main branch has been failing for the
past hour on a Docker build step. It needs diagnosing now. The roster's
infra/CI specialist is already working on an unrelated production incident
and will not free up for several hours. There is exactly one task.

Respond with only a JSON object, no other text, of the form
{"best_next_step": "...", "specialist_this_task_belongs_to": "..."} where:
- best_next_step is exactly one of: "dispatch_code_writer",
  "dispatch_orchestrator", "handle_in_main_thread", "wait_for_specialist"
- specialist_this_task_belongs_to is the roster agent name whose
  description covers this task, regardless of their availability.`
	return testkit.Test{
		ID:          "agent-deleg-capacity-trap",
		Category:    "agents",
		Subcategory: "delegation",
		Description: "Route a task whose specialist is busy: no force-fit to a mismatched agent, no orchestrator for a single task, and still name the correct owning specialist",
		Prompt:      prompt,
		// ground truth: the task is diagnosing a failing CI build, which
		// the roster assigns to devops-debugger ("Diagnoses failing
		// deployments, CI pipelines, and infrastructure incidents"). It is
		// busy, so the remaining options are judged against the roster:
		// code-writer "Implements new features or bug fixes by writing and
		// editing source code directly" - a generalist forced onto infra
		// diagnosis; orchestrator "Plans and sequences a multi-agent
		// workflow" - there is one task, nothing to sequence; waiting on a
		// specialist that is hours away while CI is down blocks delivery
		// for no gain. A single task with its specialist unavailable is
		// handled directly (handle_in_main_thread). The owning specialist
		// on the roster is unambiguously devops-debugger.
		Eval: eval.Mean(
			eval.JSONField[string]("best_next_step", "handle_in_main_thread"),
			eval.JSONField[string]("specialist_this_task_belongs_to", "devops-debugger"),
		),
	}
}

// satDelegNearMissRosterTest: 4-task mapping where every task has a
// same-vocabulary distractor (docs vs web lookup, release notes vs prose
// docs, diff review vs security review).
func satDelegNearMissRosterTest() testkit.Test {
	prompt := `You are coordinating agents. Specialist roster:

` + delegRoster + `

Map each task to exactly one roster agent - the one whose roster
description covers it. Tasks:
- task1: "Update the API guide in docs/ so it matches the endpoints as
  they exist in the repository today."
- task2: "Determine from the vendor's public pricing page which of their
  plans includes SSO."
- task3: "Produce the v2.3 release notes by generating the changelog
  section and tagging the release."
- task4: "Audit the auth handler for vulnerabilities before it merges."

Respond with only a JSON object of the form
{"task1": "...", "task2": "...", "task3": "...", "task4": "..."}
where each value is exactly one roster agent name.`
	return testkit.Test{
		ID:          "agent-deleg-near-miss-roster",
		Category:    "agents",
		Subcategory: "delegation",
		Description: "Route 4 tasks whose phrasing overlaps two roster descriptions each (repo docs vs web lookup, changelog vs prose docs, diff review vs security audit)",
		Prompt:      prompt,
		// ground truth, per the delegRoster descriptions:
		// task1 -> docs-writer: it writes/updates prose documentation
		// ("READMEs, guides, design docs") from repository content.
		// web-researcher is wrong: "Does not read or write anything in the
		// repository" and no external search is needed.
		// task2 -> web-researcher: the answer lives on the vendor's public
		// page, "Searches the public web and summarizes external
		// information"; docs-writer is the distractor ("page" sounds like
		// documentation) but the task reads the web, not the repo.
		// task3 -> release-manager: "Manages version tagging, changelog
		// generation, and release publishing" covers exactly the phrasing
		// (generate changelog + tag). docs-writer is the distractor:
		// release notes here are the release process, not a guide.
		// task4 -> security-reviewer: "Audits code or config for security
		// vulnerabilities"; code-reviewer is the distractor ("before it
		// merges" sounds like PR review) but code-reviewer reviews
		// "correctness, style, and risk", while the stated goal is
		// vulnerabilities.
		Eval: eval.Mean(
			eval.JSONField[string]("task1", "docs-writer"),
			eval.JSONField[string]("task2", "web-researcher"),
			eval.JSONField[string]("task3", "release-manager"),
			eval.JSONField[string]("task4", "security-reviewer"),
		),
	}
}

// satPGCountStarFixture is the orders/line-items data for the join
// fan-out trap.
const satPGCountStarFixture = `CREATE TABLE orders (id bigint PRIMARY KEY, region text NOT NULL);
CREATE TABLE order_items (id bigint PRIMARY KEY,
                          order_id bigint NOT NULL REFERENCES orders(id));

INSERT INTO orders (id, region) VALUES
  (1001, 'eu'), (1002, 'eu'), (1003, 'us'),
  (1004, 'eu'), (1005, 'ap'), (1006, 'eu');

INSERT INTO order_items (id, order_id) VALUES
  (9001, 1001), (9002, 1001), (9003, 1001),
  (9004, 1002),
  (9005, 1003), (9006, 1003), (9007, 1003), (9008, 1003), (9009, 1003),
  (9010, 1004), (9011, 1004),
  (9012, 1005);`

// satPGCountStarTrapTest: COUNT(*) over an INNER JOIN counts line items,
// not orders, and the join silently drops the order with zero items. Every
// number is derivable from the fixture, so the evaluator never needs to
// re-run SQL.
func satPGCountStarTrapTest() testkit.Test {
	prompt := `Here is a PostgreSQL schema with data:

` + "```sql\n" + satPGCountStarFixture + "\n```" + `

The business question is: how many ORDERS are in region 'eu'?
A developer writes:

` + "```sql\nSELECT count(*) FROM orders o JOIN order_items oi ON oi.order_id = o.id WHERE o.region = 'eu';\n```" + `

Respond with only a JSON object, no other text:
{"orders_in_eu": <number>, "rows_the_join_query_returns": <number>,
 "count_star_answers_the_question": true or false}`
	return testkit.Test{
		ID:          "pg-count-star-vs-join",
		Category:    "databases",
		Subcategory: "postgres",
		Description: "Reason about COUNT(*) over an INNER JOIN against a fixed fixture: join fan-out inflates the count and the join drops the childless order",
		Prompt:      prompt,
		// ground truth: orders with region 'eu' are 1001, 1002, 1004, 1006
		// (INSERTs above) = 4. The join emits one row per order_item:
		// 1001 -> 3 items (9001-9003), 1002 -> 1 (9004), 1004 -> 2 (9010,
		// 9011), and 1006 -> 0 items, so the INNER JOIN drops it entirely.
		// count(*) therefore returns 3+1+2 = 6. 6 != 4, so the count(*)
		// query does not answer the question (EXISTS or COUNT(DISTINCT
		// o.id) would return 4).
		Eval: eval.Mean(
			eval.JSONField[int]("orders_in_eu", 4),
			eval.JSONField[int]("rows_the_join_query_returns", 6),
			eval.JSONField[bool]("count_star_answers_the_question", false),
		),
	}
}

// satPGTimeoutTrapTest: two sessions, two timeouts, one rule each. The
// near-miss is applying idle_in_transaction_session_timeout to a busy
// session (or statement_timeout to an idle one), and starting the idle
// clock at BEGIN instead of at the last statement's completion.
func satPGTimeoutTrapTest() testkit.Test {
	prompt := `A PostgreSQL server is configured with:

` + "```\nstatement_timeout = '30s'\nidle_in_transaction_session_timeout = '5s'\n```" + `

Two sessions on this server:
- Session A: BEGIN at 11:00:00, runs statements until 11:00:40 (the last
  one finishes then), and has been waiting for client input ever since.
  The transaction is still open.
- Session B: BEGIN at 11:00:53, starts one long-running query at 11:00:55
  which is still executing at 11:01:00.

Assume no client input arrives and no query finishes after 11:01:00.
Respond with only a JSON object, no other text:
{"session_a_killed_at": "HH:MM:SS", "session_a_rule": "...",
 "session_b_killed_at": "HH:MM:SS", "session_b_rule": "..."}
where each time is in UTC and each rule is exactly one of:
"statement_timeout", "idle_in_transaction_timeout", "not_killed".`
	return testkit.Test{
		ID:          "pg-timeout-trap",
		Category:    "databases",
		Subcategory: "postgres",
		Description: "Apply statement_timeout and idle_in_transaction_session_timeout to the right sessions: idle clock starts at the last statement's completion, not BEGIN, and never applies to a session executing a statement",
		Prompt:      prompt,
		// ground truth: PostgreSQL's idle_in_transaction_session_timeout
		// terminates a session "idle ... within an open transaction" -
		// the clock starts when the session goes idle in transaction,
		// which for A is 11:00:40 (its last statement finished then), so
		// A is killed at 11:00:45 - not 11:00:05, since A was executing
		// statements from 11:00:00 to 11:00:40. A is not subject to
		// statement_timeout: no statement is running. B is idle in
		// transaction only from 11:00:53 to 11:00:55 (2s, under the 5s
		// cap) and then EXECUTES a statement, so the idle timeout no
		// longer applies; statement_timeout bounds a single running
		// statement: 11:00:55 + 30s = 11:01:25.
		Eval: eval.Mean(
			eval.JSONField[string]("session_a_killed_at", "11:00:45"),
			eval.JSONField[string]("session_a_rule", "idle_in_transaction_timeout"),
			eval.JSONField[string]("session_b_killed_at", "11:01:25"),
			eval.JSONField[string]("session_b_rule", "statement_timeout"),
		),
	}
}

// satWebRobotsFixture carries two rule groups and a longest-match pair.
const satWebRobotsFixture = `User-agent: *
Disallow: /cdn-cgi/
Allow: /cdn-cgi/style.css
Sitemap: https://example.com/sitemap.xml

User-agent: GoogleOther
Disallow: /`

// satWebRobotsUAScopeTest: GoogleOther's rules do not apply to Googlebot,
// and within Googlebot's group the longest path match decides. The
// near-miss is answering from the GoogleOther block alone, and answering
// the Allow/Disallow pair by document order instead of path length.
func satWebRobotsUAScopeTest() testkit.Test {
	prompt := `A site serves this robots.txt at https://example.com/robots.txt:

` + "```\n" + satWebRobotsFixture + "\n```" + `

Consider a GET request for https://example.com/cdn-cgi/style.css.
Respond with only a JSON object, no other text:
{"googlebot_allowed": true or false, "googleother_allowed": true or false,
 "deciding_rule_for_googlebot": "allow" or "disallow" or "no_rule"}`
	return testkit.Test{
		ID:          "web-robots-ua-scope-trap",
		Category:    "research",
		Subcategory: "web",
		Description: "robots.txt group matching across two User-agent blocks plus longest-path-match precedence - a separate group's Disallow does not apply to the other crawler",
		Prompt:      prompt,
		// ground truth: per RFC 9309, a crawler only applies the rule
		// group(s) whose User-agent matches it, and within a group the
		// path pattern with the LONGEST match wins. Googlebot does not
		// match the GoogleOther group, so only the '*' group applies:
		// the URL matches Disallow /cdn-cgi/ (9 chars) and Allow
		// /cdn-cgi/style.css (18 chars); 18 > 9, so Allow wins ->
		// googlebot_allowed true, deciding_rule_for_googlebot "allow".
		// GoogleOther matches its own group, where "Disallow: /" covers
		// every path -> googleother_allowed false.
		Eval: eval.Mean(
			eval.JSONField[bool]("googlebot_allowed", true),
			eval.JSONField[bool]("googleother_allowed", false),
			eval.JSONField[string]("deciding_rule_for_googlebot", "allow"),
		),
	}
}

// satWebHreflangSelfTest: an hreflang set missing its self-reference and
// x-default's sentinel status. The near-miss is counting the canonical
// link as the hreflang self-reference, and treating x-default as a
// language.
func satWebHreflangSelfTest() testkit.Test {
	prompt := `The page https://example.com/en-us/ serves this head section:

` + "```html\n" + `<html lang="en-US">
<head>
  <link rel="alternate" hreflang="en-AU" href="https://example.com/en-au/">
  <link rel="alternate" hreflang="en-GB" href="https://example.com/en-gb/">
  <link rel="alternate" hreflang="x-default" href="https://example.com/">
  <link rel="canonical" href="https://example.com/en-us/">
</head>` + "\n```" + `

Google's hreflang rule: an hreflang group must include a self-reference -
an hreflang annotation whose href is the page itself - and every
annotation it lists must point back reciprocally.
Respond with only a JSON object, no other text:
{"missing_self_reference_for": "the BCP 47 language tag of the page that
has no hreflang annotation pointing at it", "self_reference_present":
true or false, "canonical_counts_as_hreflang_self_reference": true or
false, "x_default_is_a_language_tag": true or false}`
	return testkit.Test{
		ID:          "web-hreflang-self-reference-trap",
		Category:    "research",
		Subcategory: "web",
		Description: "hreflang self-reference validation: the page's own language has no annotation, rel=canonical is a different mechanism, and x-default is a sentinel, not a BCP 47 tag",
		Prompt:      prompt,
		// ground truth: the page is en-US (lang attribute and canonical
		// URL agree). The hreflang annotations cover en-AU, en-GB,
		// x-default only - no annotation has href
		// https://example.com/en-us/, so the missing self-reference tag
		// is "en-US" and self_reference_present is false.
		// canonical_counts_as_hreflang_self_reference is false: rel
		// -canonical deduplicates URLs for indexing; hreflang self-
		// reference is a separate signal on rel=alternate link elements
		// - the canonical matching the page URL does not satisfy it.
		// x_default_is_a_language_tag is false: x-default is Google's
		// sentinel for "none of the specific languages", not a BCP 47
		// tag (it carries no language subtag; the other two do).
		Eval: eval.Mean(
			eval.JSONField[string]("missing_self_reference_for", "en-US"),
			eval.JSONField[bool]("self_reference_present", false),
			eval.JSONField[bool]("canonical_counts_as_hreflang_self_reference", false),
			eval.JSONField[bool]("x_default_is_a_language_tag", false),
		),
	}
}

// satAppsecORMFixture shows three SQLAlchemy call sites; only one builds
// SQL by concatenation before wrapping it in text().
const satAppsecORMFixture = `from sqlalchemy import text

def get_user(conn, name, team):
    sql = text("SELECT * FROM users WHERE name = :name AND team = :team")
    return conn.execute(sql, {"name": name, "team": team}).fetchone()

def search_users(conn, pattern):
    # callers pass the raw 'q' query-parameter straight in
    sql = text("SELECT * FROM users WHERE bio LIKE '%" + pattern + "%'")
    return conn.execute(sql).fetchall()

def list_orders(conn, status):
    return conn.execute(
        text("SELECT * FROM orders WHERE status = :s"), {"s": status}
    ).fetchall()`

// satAppsecORMTrapTest: SQLAlchemy text() is a wrapper, not a sanitizer -
// the concatenation already fused attacker input into the SQL string
// before text() ever sees it. The near-miss is "ORM == safe".
func satAppsecORMTrapTest() testkit.Test {
	prompt := `Flask application (the 'q' request parameter is attacker-controlled):

` + "```python\n" + satAppsecORMFixture + "\n```" + `

Respond with only a JSON object, no other text:
{"injectable_function": "one of get_user | search_users | list_orders",
 "text_wrapper_neutralizes_concatenation": true or false}`
	return testkit.Test{
		ID:          "sec-orm-interpolation-trap",
		Category:    "security",
		Subcategory: "appsec",
		Description: "Spot SQL injection inside a SQLAlchemy text() call built by string concatenation - the ORM wrapper neutralizes nothing once the string is already fused",
		Prompt:      prompt,
		// ground truth: get_user and list_orders pass user values as
		// bound parameters (:name/:team, :s) - the driver sends values
		// out-of-band, so no injection. search_users builds the SQL
		// string with + pattern + BEFORE text() wraps it: by the time
		// text() sees it, the attacker's input is part of the statement
		// (pattern = "x%' OR 1=1 -- " breaks out of the LIKE literal).
		// text() marks a string as raw SQL for the ORM; it never
		// parameterizes or escapes - text_wrapper_neutralizes_
		// concatenation is false.
		Eval: eval.Mean(
			eval.JSONField[string]("injectable_function", "search_users"),
			eval.JSONField[bool]("text_wrapper_neutralizes_concatenation", false),
		),
	}
}

// satAppsecPOSTFixture shows a logout control where the visible anchor is
// a decoy - the actual request is the auto-submitted POST form.
const satAppsecPOSTFixture = `<a href="#" onclick="document.getElementById('f').submit(); return false;">
  Log out
</a>
<form id="f" action="/api/account/logout" method="post">
  <input type="hidden" name="confirm" value="true">
</form>`

// satAppsecPOSTTrapTest: the app switched a token-less state-changing
// endpoint from GET to POST as its whole CSRF mitigation. Tests whether
// the model knows POST alone is not a defense (cross-site <form
// method=post> auto-submits with cookies), even when the page only ever
// submits it via JavaScript.
func satAppsecPOSTTrapTest() testkit.Test {
	prompt := `A session-authenticated web app renders this logout control. The session
cookie is SameSite=None; Secure (sent with every cross-site request), and
there is no CSRF token anywhere in the request.

` + "```\n" + satAppsecPOSTFixture + "\n```" + `

The developer's note: "I moved logout from GET to POST, so CSRF no longer
applies - only JavaScript on THIS page submits the form."

Assume an attacker page on evil.example hosts its own form with
action="https://app.example/api/account/logout", method="post". The victim
visits evil.example but clicks nothing and types nothing there.

Respond with only a JSON object, no other text:
{"attacker_form_reaches_logout": true or false,
 "post_method_alone_prevents_csrf": true or false,
 "attacker_needs_javascript": true or false}`
	return testkit.Test{
		ID:          "sec-csrf-post-trap",
		Category:    "security",
		Subcategory: "appsec",
		Description: "CSRF after a GET-to-POST switch without a token: cross-site form auto-submit carries the cookie, so the method is not the defense; the attacker needs one line of JS, not knowledge of the page's DOM",
		Prompt:      prompt,
		// ground truth: SameSite=None means the browser attaches the
		// session cookie to the attacker's cross-site POST, and there is
		// no token, so the request is accepted as the victim ->
		// attacker_form_reaches_logout true; the server authenticates by
		// cookie, not by which page submitted the form.
		// post_method_alone_prevents_csrf false: method only restricts
		// bookmark/simple-link attacks; a hosted <form method=post>
		// replays the state change. (With SameSite=Lax a cross-site POST
		// would instead drop the cookie - the prompt pins SameSite=None
		// so the answer is forced.)
		// attacker_needs_javascript true: the prompt pins "clicks nothing",
		// and without JavaScript a hosted form only submits when the victim
		// presses its own submit button - a cross-site <img>/<a> cannot POST
		// with the cookie context this request needs, and <meta
		// http-equiv="refresh"> cannot carry a POST body. The silent attack
		// requires form.submit().
		Eval: eval.Mean(
			eval.JSONField[bool]("attacker_form_reaches_logout", true),
			eval.JSONField[bool]("post_method_alone_prevents_csrf", false),
			eval.JSONField[bool]("attacker_needs_javascript", true),
		),
	}
}

// satRotationFixture gives the three durations for the rotation
// deadline math.
const satRotationFixture = `Internal CA certificate expires:   2026-09-30 18:00 UTC
Worst realistic time to mint a replacement cert once rotation starts: 10 h
After the new cert is deployed, the reverse proxy's graceful-reload
window (old workers serve the old cert for up to this long after the
reload is issued):                6 h`

// satSecretsRotationMathTest: latest safe start time = expiry minus the
// sum of both sequential windows. The near-miss is subtracting only the
// minting time and forgetting the propagation tail.
func satSecretsRotationMathTest() testkit.Test {
	prompt := `Certificate rotation facts for an internal service:

` + "```\n" + satRotationFixture + "\n```" + `

Respond with only a JSON object, no other text:
{"total_lead_time_hours": <number>, "latest_start": "YYYY-MM-DD HH:MM",
 "safe_if_started_2026_09_30_08_00": true or false}
(Use UTC throughout. latest_start is the latest time rotation can START
and still have the old certificate fully replaced everywhere before it
expires.)`
	return testkit.Test{
		ID:          "sec-rotation-window-math",
		Category:    "security",
		Subcategory: "secrets",
		Description: "Certificate rotation deadline math: lead time is minting plus propagation in series, and starting late leaves the old cert serving past expiry",
		Prompt:      prompt,
		// ground truth: the two windows are sequential - a replacement
		// can only start propagating once it exists: 10 h + 6 h = 16 h
		// worst-case lead time. 2026-09-30 18:00 minus 16 h =
		// 2026-09-30 02:00 latest start. Starting at 08:00 finishes at
		// the worst at 08:00 + 16 h = 2026-10-01 00:00, six hours after
		// the old cert expired -> not safe.
		Eval: eval.Mean(
			eval.JSONField[int]("total_lead_time_hours", 16),
			eval.JSONField[string]("latest_start", "2026-09-30 02:00"),
			eval.JSONField[bool]("safe_if_started_2026_09_30_08_00", false),
		),
	}
}

// satErrorPathFixture traces a password hash from a raise through an
// error handler into a log line, while the client only sees a generic
// response body.
const satErrorPathFixture = `1| def login(user, password):
2|     row = db.get_user(user)
3|     if not verify(password, row.hash):
4|         raise AuthError(f"invalid credentials for user={user} "
5|                         f"hash={row.hash}")
6|     return token_for(user)
7|
8| @app.errorhandler(AuthError)
9| def on_auth_error(exc):
10|     app.logger.error(str(exc))
11|     return "login failed", 401`

// satSecretsErrorPathTest: the secret (password hash) is interpolated at
// the raise, survives the handler's str(exc), and lands in the log even
// though the HTTP response is sanitized. The near-miss is "the client
// never sees it, so it's fine", and the reverse trap of flagging the
// attacker-supplied username as the secret.
func satSecretsErrorPathTest() testkit.Test {
	prompt := `Flask login flow (numbers are source lines):

` + "```\n" + satErrorPathFixture + "\n```" + `

row.hash is the stored password hash (sensitive). user comes straight
from the login request body.
Respond with only a JSON object, no other text:
{"hash_reaches_server_logs": true or false, "hash_visible_in_http_response":
true or false, "sensitive_value_in_message": "one of user | hash | none"}`
	return testkit.Test{
		ID:          "sec-error-path-leak-trap",
		Category:    "security",
		Subcategory: "secrets",
		Description: "Trace a password hash through raise -> errorhandler -> logger.error: the client response is sanitized but the log line is not, and the echoed user field is attacker-known",
		Prompt:      prompt,
		// ground truth: the raise on lines 4-5 interpolates row.hash into
		// the exception message; the handler on line 10 logs str(exc) -
		// the full message - so the hash reaches the server logs: true.
		// The handler returns only the literal "login failed" (line 11),
		// never str(exc), so the hash is not visible in the HTTP
		// response: false. Between user (attacker-supplied in the request
		// body, already known to its owner) and hash (sensitive), the
		// sensitive interpolated value is the hash.
		Eval: eval.Mean(
			eval.JSONField[bool]("hash_reaches_server_logs", true),
			eval.JSONField[bool]("hash_visible_in_http_response", false),
			eval.JSONField[string]("sensitive_value_in_message", "hash"),
		),
	}
}
