package tests

import (
	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

// webCORSPreflightExchange is the inline preflight request/response
// exchange for webCORSMissingHeaderTest.
const webCORSPreflightExchange = `Browser preflight request (from https://app.example.com):
OPTIONS /data HTTP/1.1
Host: api.example.com
Origin: https://app.example.com
Access-Control-Request-Method: PUT
Access-Control-Request-Headers: X-Custom-Auth

Server's preflight response:
HTTP/1.1 204 No Content
Access-Control-Allow-Origin: https://app.example.com
Access-Control-Allow-Methods: GET, POST, PUT`

// webCORSMissingHeaderPattern requires the literal token
// "Access-Control-Allow-Headers" (case-insensitive) anywhere in the
// response, accepting it with or without a trailing colon or surrounding
// prose.
const webCORSMissingHeaderPattern = `(?i)access-control-allow-headers`

// webCORSMissingHeaderTest: identify the single missing CORS response
// header that will cause the browser to block the request.
//
// ground truth: the browser sent Access-Control-Request-Headers:
// X-Custom-Auth in the preflight, but the server's response never echoes
// that header name back in an Access-Control-Allow-Headers response
// header, so the browser blocks the actual PUT request even though the
// origin and method are both explicitly allowed.
func webCORSMissingHeaderTest() testkit.Test {
	prompt := `Here is a CORS preflight exchange:

` + webCORSPreflightExchange + `

The browser will block the actual PUT request that follows this
preflight. Which single response header is missing from the server's
preflight response, causing the block? Respond with only the header name,
nothing else.`

	return testkit.Test{
		ID:          "web-cors-missing-header",
		Category:    "research",
		Subcategory: "web",
		Description: "Identify the single missing CORS response header that causes a browser to block a preflighted request.",
		Prompt:      prompt,
		Eval:        eval.Regex(webCORSMissingHeaderPattern),
	}
}

// webSecurityHeadersResponse is the inline HTTP response header block for
// webSecurityHeadersAuditTest.
const webSecurityHeadersResponse = `HTTP/1.1 200 OK
Content-Type: text/html; charset=utf-8
Cache-Control: no-cache
Set-Cookie: session=abc123; HttpOnly
Server: nginx
X-Content-Type-Options: nosniff
Referrer-Policy: strict-origin-when-cross-origin`

// webSecurityHeadersBaseline is the recommended baseline of security
// headers checked against webSecurityHeadersResponse.
const webSecurityHeadersBaseline = `Content-Security-Policy, Strict-Transport-Security,
X-Content-Type-Options, X-Frame-Options, Referrer-Policy`

// webSecurityHeadersAuditTest: identify which baseline security headers
// are missing from an inline HTTP response.
//
// ground truth: of the 5 baseline headers, X-Content-Type-Options and
// Referrer-Policy are present in the response; Content-Security-Policy,
// Strict-Transport-Security, and X-Frame-Options are absent.
func webSecurityHeadersAuditTest() testkit.Test {
	prompt := `Here is an HTTP response's headers:

` + webSecurityHeadersResponse + `

Here is the recommended baseline of security headers:

` + webSecurityHeadersBaseline + `

Which of the baseline headers are missing from the response above?
Respond with only a JSON array of the missing header names.`

	return testkit.Test{
		ID:          "web-security-headers-audit",
		Category:    "research",
		Subcategory: "web",
		Description: "Identify which baseline security headers are missing from an inline HTTP response, given a partial-compliance fixture.",
		Prompt:      prompt,
		Eval:        eval.JSONStringSet([]string{"Content-Security-Policy", "Strict-Transport-Security", "X-Frame-Options"}),
	}
}
