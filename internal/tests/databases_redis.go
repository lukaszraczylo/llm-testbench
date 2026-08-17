package tests

import (
	"context"
	"regexp"
	"strings"

	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

// registerDBRedisTests registers every databases/redis test.
func registerDBRedisTests(r *testkit.Registry) {
	r.Register(dbRedisStructureChoiceTest())
	r.Register(dbRedisTTLEvictionTest())
	r.Register(dbRedisIncrAtomicityTest())
	r.Register(dbRedisMultiVsPipelineTest())
	r.Register(dbRedisScanVsKeysTest())
	r.Register(dbRedisPubSubDeliveryTest())
	r.Register(dbRedisLuaAtomicityTest())
	r.Register(dbRedisMemoryEstimateTest())
	r.Register(dbRedisCacheStampedeTest())
	r.Register(dbRedisKeyspaceAntiPatternTest())
}

// dbRedisStructureScenarios lists the 4 inline scenarios for
// dbRedisStructureChoiceTest.
const dbRedisStructureScenarios = `a: "A leaderboard of user scores that must support efficient range and
   rank queries (e.g. 'give me the top 10' or 'what rank is this user')."
b: "Counting approximately how many unique visitors hit the site today,
   at a scale of tens of millions of visitors, using minimal memory."
c: "A user's session data as a group of named fields (name, plan,
   last_seen) that must each be readable and updatable individually
   without rewriting the whole session."
d: "A FIFO job queue where worker processes block until a new job
   arrives, then pop and process jobs in the order they were pushed."`

// dbRedisStructureChoiceTest: pick the best-fit Redis data structure for
// each of 4 inline usage scenarios, from a closed vocabulary.
//
// ground truth: (a) a leaderboard needing ranked range queries is exactly
// what a sorted set (ZSET, score-ordered) provides. (b) approximate unique
// counting at large scale with minimal memory is HyperLogLog's purpose
// (a small fixed-size probabilistic structure), not an exact set which
// would use memory proportional to visitor count. (c) named, individually
// updatable fields is a hash, not a single string blob that would need a
// full read-modify-write. (d) a FIFO queue with blocking pop is a list
// (LPUSH/BRPOP), not a set (unordered, no blocking pop) or a stream
// (heavier, consumer-group semantics not needed here).
func dbRedisStructureChoiceTest() testkit.Test {
	prompt := `For each of these 4 scenarios, pick the single best-fit Redis
data structure from exactly this vocabulary: string, list, hash, set,
sorted-set, hyperloglog, stream, bitmap.

` + dbRedisStructureScenarios + `

Respond with only a JSON object mapping each letter to one structure name:
{"a":"...","b":"...","c":"...","d":"..."}`

	evaluator := eval.Mean(
		eval.JSONField("a", "sorted-set"),
		eval.JSONField("b", "hyperloglog"),
		eval.JSONField("c", "hash"),
		eval.JSONField("d", "list"),
	)

	return testkit.Test{
		ID:          "redis-structure-choice",
		Category:    "databases",
		Subcategory: "redis",
		Description: "Pick the best-fit Redis data structure (sorted-set, hyperloglog, hash, list) for 4 inline usage scenarios from a closed vocabulary.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// dbRedisTTLEvictionTest: determine what happens to a write under
// maxmemory-policy=noeviction when Redis is already at maxmemory.
//
// ground truth: noeviction means Redis will never evict a key to make
// room. A write that would grow memory usage while already at maxmemory
// is instead rejected outright with an OOM error - the write does not
// silently succeed and no key is evicted to make space.
func dbRedisTTLEvictionTest() testkit.Test {
	prompt := `A Redis instance has maxmemory-policy set to "noeviction" and
is currently at maxmemory. A client sends a write command that would
increase memory usage further.

What happens to that write? Respond with only one word: reject or evict.`

	return testkit.Test{
		ID:          "redis-ttl-eviction",
		Category:    "databases",
		Subcategory: "redis",
		Description: "Determine that a write is rejected (not silently allowed, and nothing evicted) under maxmemory-policy=noeviction at maxmemory.",
		Prompt:      prompt,
		Eval:        dbExactAnswer("reject"),
	}
}

// dbRedisIncrAtomicityTest: contrast INCR's atomic guarantee against the
// race in a manual GET-then-SET increment pattern.
//
// ground truth: INCR is a single atomic server-side operation, so 100
// concurrent INCR calls on a counter starting at 0 are guaranteed to leave
// it at exactly 100, with no lost updates. The GET-then-SET pattern is two
// separate round trips per client; two clients can both GET the same
// value before either SETs, so one client's increment can be silently
// overwritten - the final value after 100 concurrent GET/SET increments is
// not guaranteed to reach 100.
func dbRedisIncrAtomicityTest() testkit.Test {
	prompt := `A Redis counter starts at 0. 100 clients each run exactly one
of the two patterns below, concurrently:

Pattern A: each client calls INCR counter once.
Pattern B: each client calls GET counter, adds 1 in its own application
code, then calls SET counter <new value> once.

What is the guaranteed final value of the counter after Pattern A's 100
calls, and is Pattern B's final value guaranteed to also reach that same
total? Respond with only a JSON object:
{"incr_final":<number>,"getset_race_safe":true|false}`

	evaluator := eval.Mean(
		eval.JSONField("incr_final", 100),
		eval.JSONField("getset_race_safe", false),
	)

	return testkit.Test{
		ID:          "redis-incr-atomicity",
		Category:    "databases",
		Subcategory: "redis",
		Description: "Contrast INCR's guaranteed atomic result (100) against the race-prone GET-then-SET increment pattern (not guaranteed to reach 100).",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// dbRedisMultiVsPipelineTest: confirm that plain pipelining, unlike
// MULTI/EXEC, gives no atomicity/isolation guarantee against interleaving
// from other clients.
//
// ground truth: pipelining is purely a network optimization - batching
// several commands into one round trip - with no guarantee that the
// server executes them back-to-back relative to other connected clients.
// MULTI/EXEC (a transaction) is what guarantees no other client's command
// can execute in between the queued commands.
func dbRedisMultiVsPipelineTest() testkit.Test {
	prompt := `Does plain command pipelining (batching several commands into
one network round trip, without wrapping them in MULTI/EXEC) guarantee
that no other client's command can execute on the server in between the
pipelined commands? Respond with only "yes" or "no".`

	return testkit.Test{
		ID:          "redis-multi-vs-pipeline",
		Category:    "databases",
		Subcategory: "redis",
		Description: "Confirm plain pipelining (unlike MULTI/EXEC) gives no guarantee against another client's commands interleaving.",
		Prompt:      prompt,
		Eval:        dbExactAnswer("no"),
	}
}

// dbKeysCommandPattern matches an explicit mention of the Redis KEYS
// command actually being invoked - "KEYS *" (its most common prod-danger
// form) or a backtick-quoted `KEYS` - not a generic use of the English
// word "keys" elsewhere in prose ("the keys to fixing this...").
var dbKeysCommandPattern = regexp.MustCompile("(?:`KEYS`|\\bKEYS\\s*\\*)")

// dbNegationCuePattern matches a word that turns a mention of the KEYS
// command into a warning against running it in production, rather than an
// instruction to run it.
var dbNegationCuePattern = regexp.MustCompile(`(?i)\b(don'?t|do not|never|avoid|instead of|not|shouldn'?t|should not|rather than|without|no need|danger|dangerous)\b`)

// dbNegationWindow is how many characters before a KEYS-command mention
// are searched for a negation cue.
const dbNegationWindow = 60

// dbNegationWindowStart returns the earliest byte offset in response to
// search for a negation cue before a KEYS-command mention starting at
// start: the current line, extended back at most dbNegationWindow
// characters.
func dbNegationWindowStart(response string, start int) int {
	lineStart := strings.LastIndexByte(response[:start], '\n') + 1
	return max(start-dbNegationWindow, lineStart)
}

// dbNoBareKeysInProd scores full credit unless the response instructs
// running "KEYS *" against a production instance without any negating
// context (a warning not to, or an instruction to use SCAN instead is
// fine). This is deliberately not eval.NotContains, which would also zero
// out the best possible answer: one that correctly explains why NOT to run
// KEYS in production.
func dbNoBareKeysInProd() eval.Evaluator {
	return eval.EvaluatorFunc(func(_ context.Context, response string) eval.Score {
		matches := dbKeysCommandPattern.FindAllStringIndex(response, -1)
		if len(matches) == 0 {
			return eval.Score{Value: 1, Detail: "no mention of running the KEYS command"}
		}
		for _, loc := range matches {
			start := loc[0]
			windowStart := dbNegationWindowStart(response, start)
			if !dbNegationCuePattern.MatchString(response[windowStart:start]) {
				return eval.Score{Value: 0, Detail: "unnegated mention of running the KEYS command"}
			}
		}
		return eval.Score{Value: 1, Detail: "every mention of KEYS is negated"}
	})
}

// dbRedisScanVsKeysTest: require SCAN over KEYS for a hot-path production
// lookup, using a negation-aware guard so an answer that correctly warns
// against KEYS is not penalized for mentioning it.
func dbRedisScanVsKeysTest() testkit.Test {
	prompt := `A request handler that runs thousands of times per second
against a production Redis instance holding millions of keys currently
calls "KEYS *" to find every key matching a pattern.

What is wrong with this in production, and what command should be used
instead to iterate the keyspace safely?`

	evaluator := eval.All(
		eval.W(dbNoBareKeysInProd(), 2),
		eval.W(eval.ContainsAny("SCAN"), 2),
	)

	return testkit.Test{
		ID:          "redis-scan-vs-keys",
		Category:    "databases",
		Subcategory: "redis",
		Description: "Require the SCAN cursor over KEYS * for a hot-path production lookup, without penalizing a negated warning against KEYS.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// dbRedisPubSubDeliveryTest: confirm Redis pub/sub gives no delivery
// guarantee to a disconnected subscriber.
//
// ground truth: Redis pub/sub is fire-and-forget with no backlog or
// replay - a message published while a subscriber is disconnected is
// simply not delivered to it; reconnecting does not retrieve messages
// missed while offline.
func dbRedisPubSubDeliveryTest() testkit.Test {
	prompt := `A subscriber is disconnected (e.g. a brief network blip) at
the exact moment a message is published on a channel it normally
subscribes to. After the subscriber reconnects, will it receive that
specific message it missed while disconnected? Respond with only "yes" or
"no".`

	return testkit.Test{
		ID:          "redis-pubsub-delivery",
		Category:    "databases",
		Subcategory: "redis",
		Description: "Confirm Redis pub/sub gives no delivery guarantee (no backlog/replay) to a subscriber that was disconnected when a message was published.",
		Prompt:      prompt,
		Eval:        dbExactAnswer("no"),
	}
}

// dbRedisLuaAtomicityTest: confirm a Lua script run via EVAL executes
// atomically with no other client command interleaved.
//
// ground truth: Redis executes commands (and an entire EVAL script) on a
// single command-processing thread; while one client's script is running,
// the server does not process any other client's command, so a script is
// guaranteed to run start-to-finish with nothing interleaved.
func dbRedisLuaAtomicityTest() testkit.Test {
	prompt := `While one client's Lua script is executing via EVAL, can any
other client's command run on the Redis server before that script
finishes? Respond with only "yes" or "no".`

	return testkit.Test{
		ID:          "redis-lua-atomicity",
		Category:    "databases",
		Subcategory: "redis",
		Description: "Confirm a Lua script run via EVAL executes atomically, with no other client command able to interleave.",
		Prompt:      prompt,
		Eval:        dbExactAnswer("no"),
	}
}

// dbRedisMemoryEstimateWant is the total estimated bytes computed by the
// formula given inline in dbRedisMemoryEstimateTest's prompt.
//
// ground truth: total_bytes = count * (key_bytes + value_bytes +
// overhead_bytes) = 200000 * (20 + 100 + 56) = 200000 * 176 = 35,200,000.
// databases_redis_test.go recomputes this independently with the same
// arithmetic written out plainly.
const dbRedisMemoryEstimateWant = 200000 * (20 + 100 + 56)

// dbRedisMemoryEstimateTest: compute total memory bytes for an inline
// dataset from a formula given entirely in the prompt.
func dbRedisMemoryEstimateTest() testkit.Test {
	prompt := `Estimate total memory, in bytes, to store a Redis dataset of
200,000 string key-value pairs, where each key name is 20 bytes, each
value is 100 bytes, and Redis's per-key-value overhead (hash table entry
plus object headers) is estimated at 56 bytes. Use this formula:

total_bytes = count * (key_bytes + value_bytes + overhead_bytes)

Respond with only the number.`

	return testkit.Test{
		ID:          "redis-memory-estimate",
		Category:    "databases",
		Subcategory: "redis",
		Description: "Compute total memory bytes for a 200,000-key dataset from an inline per-key-value formula and byte sizes.",
		Prompt:      prompt,
		Eval:        eval.Numeric(eval.ExtractLastNumber[int], dbRedisMemoryEstimateWant, 0),
	}
}

// dbRedisStampedeCandidates lists the 4 candidate mitigation techniques
// for dbRedisCacheStampedeTest, mixing genuinely valid mitigations with
// one plausible-sounding non-mitigation.
const dbRedisStampedeCandidates = `- mutex_lock_on_regenerate: only one client regenerates a missing/expired
  key at a time; others wait for or read the in-flight result.
- probabilistic_early_expiration: clients probabilistically recompute a
  soon-to-expire key slightly before it actually expires, spreading
  regeneration out instead of all racing at the exact expiry moment.
- increase_replica_count: add more read replicas to the Redis deployment.
- stale_while_revalidate: serve the stale cached value immediately while
  one background request regenerates it.`

// dbRedisStampedeWant is the subset of dbRedisStampedeCandidates that
// actually mitigates cache stampede.
//
// ground truth: a cache stampede is a burst of concurrent regenerations
// racing to rebuild the same expired key against the backing store.
// mutex_lock_on_regenerate, probabilistic_early_expiration, and
// stale_while_revalidate each directly prevent that race (by
// serializing regeneration, spreading it out over time, or deferring it
// entirely). Adding more replicas does not: replicas serve reads of
// already-cached data, but every replica independently misses the same
// expired key at the same moment, so it does not reduce or serialize the
// regeneration race against the backing store at all.
var dbRedisStampedeWant = []string{
	"mutex_lock_on_regenerate",
	"probabilistic_early_expiration",
	"stale_while_revalidate",
}

func dbRedisCacheStampedeTest() testkit.Test {
	prompt := `A hot cache key expires and many concurrent requests all miss
it at once, each racing to regenerate it against the backing store (a
"cache stampede" / "thundering herd"). Here are 4 candidate mitigation
techniques:

` + dbRedisStampedeCandidates + `

Which of these candidates actually mitigate a cache stampede? Respond with
only a JSON array of the candidate names that do.`

	return testkit.Test{
		ID:          "redis-cache-stampede",
		Category:    "databases",
		Subcategory: "redis",
		Description: "Select the cache-stampede mitigations (mutex, probabilistic early expiration, stale-while-revalidate) from 4 candidates that also include a non-mitigation.",
		Prompt:      prompt,
		Eval:        eval.JSONStringSet(dbRedisStampedeWant),
	}
}

// dbRedisKeyspaceAntiPatternCode is the inline monolithic-key snippet for
// dbRedisKeyspaceAntiPatternTest.
const dbRedisKeyspaceAntiPatternCode = `def save_all_users(users):
    r.set("all_users", json.dumps(users))  # users: a list of 500,000 user dicts

def update_user(user_id, fields):
    users = json.loads(r.get("all_users"))
    for u in users:
        if u["id"] == user_id:
            u.update(fields)
    r.set("all_users", json.dumps(users))  # rewrites the entire 500,000-user blob`

// dbRedisKeyspaceAntiPatternTest: spot a monolithic single-key blob
// keyspace anti-pattern in inline code and require the per-entity hash
// fix.
//
// ground truth: every single-field update to one user rewrites a
// serialized blob of all 500,000 users under one key, an O(n) operation
// per update where n is the whole user count - a monolithic-key
// anti-pattern, not a "too many small keys" problem (there is only one
// key here) or a missing-TTL problem (nothing about expiry is shown). The
// fix is to give each user its own Hash key (e.g. user:<id>), so a field
// update touches O(1) data instead of the entire dataset.
func dbRedisKeyspaceAntiPatternTest() testkit.Test {
	prompt := `Here is code managing user data in Redis:

` + "```python\n" + dbRedisKeyspaceAntiPatternCode + "\n```" + `

What keyspace design problem does this have, and what is the correct fix?
Respond with only a JSON object:
{"problem":"<one of: monolithic-key, too-many-small-keys, missing-ttl>","fix":"<one of: hash-per-user, single-key-ok, add-ttl>"}`

	evaluator := eval.Mean(
		eval.JSONField("problem", "monolithic-key"),
		eval.JSONField("fix", "hash-per-user"),
	)

	return testkit.Test{
		ID:          "redis-keyspace-anti-pattern",
		Category:    "databases",
		Subcategory: "redis",
		Description: "Spot a monolithic single-key user blob rewritten on every update and require the per-user-hash fix.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}
