package tests

import (
	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

// registerDBPostgresTests registers every databases/postgres test.
func registerDBPostgresTests(r *testkit.Registry) {
	r.Register(dbPGExplainSeqVsIndexTest())
	r.Register(dbPGIndexChoiceTest())
	r.Register(dbPGNPlusOneTest())
	r.Register(dbPGIsolationAnomalyTest())
	r.Register(dbPGDeadlockLockOrderTest())
	r.Register(dbPGReplicaFailoverTest())
	r.Register(dbPGBloatVacuumTest())
	r.Register(dbPGPoolSizingTest())
	r.Register(dbPGPartialIndexTest())
	r.Register(dbPGListenNotifyTest())
}

// dbPGExplainSeqVsIndexPlan is the inline EXPLAIN excerpt for
// dbPGExplainSeqVsIndexTest.
const dbPGExplainSeqVsIndexPlan = `Seq Scan on events  (cost=0.00..215000.00 rows=4200000 width=8)
  Filter: (status = 'active'::text)`

// dbPGExplainSeqVsIndexTest: interpret why the planner chose a sequential
// scan over an available index, from an inline EXPLAIN plan and the table's
// row counts.
//
// ground truth: the events table holds 10,000,000 rows and the plan
// estimates 4,200,000 of them match status='active' - 42% of the table.
// Above roughly 5-10% selectivity, a sequential scan reading every page
// once beats an index scan that would revisit the heap almost as many
// times as a seq scan touches pages, so a low-selectivity filter (not a
// missing index - one exists - and not stale statistics) is why the
// planner picked Seq Scan. At a selectivity of 8,000/10,000,000 (0.08%),
// the same index would clearly be preferred instead.
func dbPGExplainSeqVsIndexTest() testkit.Test {
	prompt := `The "events" table holds 10,000,000 rows and has a btree index
on its "status" column. Here is the EXPLAIN output for
"SELECT id FROM events WHERE status = 'active';":

` + "```\n" + dbPGExplainSeqVsIndexPlan + "\n```" + `

The planner used a sequential scan even though an index on status exists.
Given the estimated row count in the plan versus the table's total row
count, what row-count property of the filter caused this choice? Also: if
instead only 8,000 of the 10,000,000 rows had status='rare' (a highly
selective filter), would the planner likely prefer the index scan instead?
Respond with only a JSON object:
{"reason":"<one of: low-selectivity, missing-index, stale-statistics>","would_index_help_at_rare_status":true|false}`

	evaluator := eval.Mean(
		eval.JSONField("reason", "low-selectivity"),
		eval.JSONField("would_index_help_at_rare_status", true),
	)

	return testkit.Test{
		ID:          "pg-explain-seq-vs-index",
		Category:    "databases",
		Subcategory: "postgres",
		Description: "Interpret an EXPLAIN plan's Seq Scan choice as a low-selectivity filter (42% of the table), not a missing index.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// dbPGIndexChoiceSchema is the inline schema + query for
// dbPGIndexChoiceTest.
const dbPGIndexChoiceSchema = `Table: orders(id bigserial, region text, order_date date, amount numeric)

Query:
SELECT id, amount FROM orders
WHERE region = 'EU' AND order_date >= '2026-01-01'
ORDER BY order_date;`

// dbPGIndexChoiceTest: pick the composite index column order (equality
// column first, then the range/sort column) that best serves an inline
// query.
//
// ground truth: region is filtered by exact equality, order_date is
// filtered by a range AND drives the ORDER BY. A composite index leads
// with the equality column so the index narrows to matching rows first,
// then with the range/sort column so those rows are already in the index's
// order needed by ORDER BY: (region, order_date). Reversing the order
// (order_date, region) forces a full scan of every order_date >= the bound
// across all regions before filtering by region.
func dbPGIndexChoiceTest() testkit.Test {
	prompt := `Here is a table schema and a query that runs frequently:

` + "```\n" + dbPGIndexChoiceSchema + "\n```" + `

Which two columns, in which order, should a single composite index be
defined on to best serve this exact query (equality filters should lead a
composite index, ahead of range/sort columns)? Respond with only a JSON
array of the two column names in index-definition order, e.g. ["a","b"].`

	return testkit.Test{
		ID:          "pg-index-choice",
		Category:    "databases",
		Subcategory: "postgres",
		Description: "Pick composite index column order (region, order_date) for an inline equality+range+sort query.",
		Prompt:      prompt,
		Eval:        eval.JSONStringArrayEquals([]string{"region", "order_date"}),
	}
}

// dbPGNPlusOneCode is the inline query-in-a-loop snippet for
// dbPGNPlusOneTest.
const dbPGNPlusOneCode = `orders = db.query("SELECT id, customer_id FROM orders WHERE status = 'pending'")
for o in orders:
    customer = db.query("SELECT name FROM customers WHERE id = %s", [o.customer_id])
    print(o.id, customer.name)`

// dbPGNPlusOneTest: spot the N+1 query pattern in an inline loop and name
// its single-query fix.
//
// ground truth: the code runs one query to fetch pending orders, then one
// additional query per order to fetch that order's customer name inside
// the loop - the classic N+1 pattern, not a Cartesian product (which would
// return duplicated rows from a single query) or a missing index (which
// would not change the query count at all). The fix is to replace the
// per-row queries with a single JOIN (or a single WHERE id = ANY(...)
// batch query) so exactly one query fetches every needed customer name.
func dbPGNPlusOneTest() testkit.Test {
	prompt := `Here is application code that fetches pending orders and their
customer names:

` + "```python\n" + dbPGNPlusOneCode + "\n```" + `

What query pattern is this, and what single-query fix addresses it?
Respond with only a JSON object:
{"pattern":"<one of: n-plus-one, missing-index, cartesian-product>","fix":"<one of: single-join-query, add-cache, add-index>"}`

	evaluator := eval.Mean(
		eval.JSONField("pattern", "n-plus-one"),
		eval.JSONField("fix", "single-join-query"),
	)

	return testkit.Test{
		ID:          "pg-n-plus-one",
		Category:    "databases",
		Subcategory: "postgres",
		Description: "Spot the N+1 query pattern in an inline per-row query loop and require a single-JOIN fix.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// dbPGIsolationAnomalySchedule is the inline two-transaction interleaved
// schedule for dbPGIsolationAnomalyTest.
const dbPGIsolationAnomalySchedule = `T1: BEGIN;
T1: SELECT balance FROM accounts WHERE id = 1;   -- returns 100
T2: BEGIN;
T2: UPDATE accounts SET balance = 50 WHERE id = 1;
T2: COMMIT;
T1: SELECT balance FROM accounts WHERE id = 1;   -- returns 50
T1: COMMIT;`

// dbPGIsolationAnomalyTest: name the isolation anomaly a READ COMMITTED
// transaction observes when its own two reads of the same row disagree
// because another transaction committed in between.
//
// ground truth: T1 reads balance=100, then, after T2 commits a change to
// the same row, T1's second read of the exact same row within the SAME
// transaction returns 50 - two reads of one row disagreeing inside one
// transaction is the definition of a non-repeatable read, not a dirty read
// (which would require reading T2's UPDATE before T2 committed) or a
// phantom read (which is about a changed set of rows matching a
// predicate, not one row's value).
func dbPGIsolationAnomalyTest() testkit.Test {
	prompt := `T1 runs under READ COMMITTED isolation. Here is an interleaved
schedule of T1 and a concurrent T2:

` + "```\n" + dbPGIsolationAnomalySchedule + "\n```" + `

Which isolation anomaly does T1 observe between its two SELECTs? Respond
with only a JSON object:
{"anomaly":"<one of: dirty-read, non-repeatable-read, phantom-read, lost-update>"}`

	return testkit.Test{
		ID:          "pg-isolation-anomaly",
		Category:    "databases",
		Subcategory: "postgres",
		Description: "Name the non-repeatable-read anomaly from an inline READ COMMITTED schedule where a row's value changes between two reads in one transaction.",
		Prompt:      prompt,
		Eval:        eval.JSONField("anomaly", "non-repeatable-read"),
	}
}

// dbPGDeadlockSchedule is the inline two-transaction opposite-lock-order
// schedule for dbPGDeadlockLockOrderTest.
const dbPGDeadlockSchedule = `T1: BEGIN;
T1: UPDATE accounts SET balance = balance - 100 WHERE id = 1;  -- T1 locks id=1
T1: UPDATE accounts SET balance = balance + 100 WHERE id = 2;  -- T1 waits: T2 holds id=2

T2: BEGIN;
T2: UPDATE accounts SET balance = balance - 50 WHERE id = 2;   -- T2 locks id=2
T2: UPDATE accounts SET balance = balance + 50 WHERE id = 1;   -- T2 waits: T1 holds id=1`

// dbPGDeadlockLockOrderTest: recognize a deadlock caused by two
// transactions locking the same two rows in opposite order, and give the
// consistent lock-acquisition order that prevents it.
//
// ground truth: T1 locks id=1 then waits on id=2 (held by T2); T2 locks
// id=2 then waits on id=1 (held by T1) - a circular wait, so Postgres'
// deadlock detector must abort one transaction. Both transactions always
// touching rows in the same order - ascending id, so id=1 before id=2 -
// eliminates the circular wait, since neither transaction would ever hold
// a higher id's lock while waiting on a lower one.
func dbPGDeadlockLockOrderTest() testkit.Test {
	prompt := `Here are two concurrent transactions on the same accounts table:

` + "```\n" + dbPGDeadlockSchedule + "\n```" + `

Do these two transactions deadlock? If so, what consistent row lock
acquisition order (by id, ascending) would both transactions need to follow
to prevent this deadlock? Respond with only a JSON object:
{"deadlock":true|false,"safe_order_ids":[<first id to lock>,<second id to lock>]}`

	evaluator := eval.Mean(
		eval.JSONField("deadlock", true),
		eval.JSONField("safe_order_ids[0]", 1),
		eval.JSONField("safe_order_ids[1]", 2),
	)

	return testkit.Test{
		ID:          "pg-deadlock-lock-order",
		Category:    "databases",
		Subcategory: "postgres",
		Description: "Recognize a deadlock from two transactions locking rows id=1/id=2 in opposite order and require the ascending-id safe order.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// dbPGReplicaFailoverTest: explain that an asynchronously-replicated commit
// can be lost when the primary crashes before the replica catches up and
// gets promoted.
//
// ground truth: with asynchronous streaming replication (no
// synchronous_commit), a transaction's COMMIT returns to the client as
// soon as it is durable on the primary, before its WAL is guaranteed to
// have reached the replica. If the primary crashes in that window and the
// replica is promoted, the promoted replica becomes the new primary
// without ever having applied that transaction's WAL - so the committed
// transaction is not visible on it and is effectively lost, unless it can
// be recovered from the old primary's WAL after the fact.
func dbPGReplicaFailoverTest() testkit.Test {
	prompt := `A Postgres primary has one streaming replica, using
asynchronous replication (synchronous_commit is not enabled). A client's
transaction COMMITs successfully on the primary. Before the WAL for that
commit reaches the replica, the primary host crashes, and the replica is
promoted to become the new primary.

Is that committed transaction guaranteed to be visible on the newly
promoted primary? Explain what happens to it and why, in one or two
sentences.`

	evaluator := eval.All(
		eval.W(eval.ContainsAny("lost", "not visible", "not guaranteed", "may be lost", "can be lost", "data loss"), 2),
		eval.W(eval.ContainsAny("async", "asynchronous"), 1),
	)

	return testkit.Test{
		ID:          "pg-replica-failover",
		Category:    "databases",
		Subcategory: "postgres",
		Description: "Explain that an asynchronously-replicated commit can be lost on failover if the primary crashes before the replica catches up.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// dbPGBloatVacuumStats is the inline pg_stat_user_tables excerpt for
// dbPGBloatVacuumTest.
const dbPGBloatVacuumStats = `relname | n_live_tup | n_dead_tup | last_autovacuum
orders  | 500000     | 4500000    | (null)`

// dbPGBloatVacuumTest: diagnose table bloat from an inline pg_stat_
// user_tables excerpt showing dead tuples vastly outnumbering live ones and
// autovacuum never having run.
//
// ground truth: n_dead_tup (4,500,000) is 9x n_live_tup (500,000) - about
// 90% of the table's physical rows are dead - and last_autovacuum is null,
// meaning autovacuum has never completed on this table. That combination
// is table bloat from accumulated dead tuples, not a missing index (which
// this fixture says nothing about) or replication lag (unrelated to
// tuple counts). The fix is to VACUUM the table (reclaiming/reusing the
// dead tuple space), not to reindex or add an index.
func dbPGBloatVacuumTest() testkit.Test {
	prompt := `Here is an excerpt of pg_stat_user_tables for one table:

` + "```\n" + dbPGBloatVacuumStats + "\n```" + `

What is the likely cause of this table's condition, and what single
maintenance action addresses it? Respond with only a JSON object:
{"cause":"<one of: bloat, missing-index, replication-lag>","action":"<one of: vacuum, reindex, add-index>"}`

	evaluator := eval.Mean(
		eval.JSONField("cause", "bloat"),
		eval.JSONField("action", "vacuum"),
	)

	return testkit.Test{
		ID:          "pg-bloat-vacuum",
		Category:    "databases",
		Subcategory: "postgres",
		Description: "Diagnose table bloat (dead tuples 9x live, autovacuum never ran) from an inline pg_stat_user_tables excerpt and require the vacuum fix.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// dbPGPoolSizingWant is the pool_size computed by the formula given inline
// in dbPGPoolSizingTest's prompt.
//
// ground truth: pool_size = (core_count * 2) + effective_spindle_count =
// (8 * 2) + 1 = 17. databases_postgres_test.go recomputes this
// independently with the same arithmetic written out plainly.
const dbPGPoolSizingWant = 8*2 + 1

// dbPGPoolSizingTest: compute a connection pool size from a formula and
// parameters both given inline in the prompt.
func dbPGPoolSizingTest() testkit.Test {
	prompt := `Use this connection pool sizing formula:

pool_size = (core_count * 2) + effective_spindle_count

A database server has core_count = 8. Its storage is a single NVMe SSD,
which this formula treats as effective_spindle_count = 1. Compute
pool_size. Respond with only the number.`

	return testkit.Test{
		ID:          "pg-pool-sizing",
		Category:    "databases",
		Subcategory: "postgres",
		Description: "Compute a connection pool size from an inline formula (core_count*2 + effective_spindle_count) and given parameters.",
		Prompt:      prompt,
		Eval:        eval.Numeric(eval.ExtractLastNumber[int], dbPGPoolSizingWant, 0),
	}
}

// dbPGPartialIndexTest: judge whether a partial index is a good fit for an
// inline query pattern where the filtered value is a small, hot subset of
// a huge table.
//
// ground truth: only 120,000 of 10,000,000 rows (1.2%) have
// status='pending', and the application's hottest query filters on exactly
// that value almost every time it queries this table. A partial index
// `ON orders(...) WHERE status = 'pending'` indexes only that small,
// frequently-queried subset - far smaller and cheaper to maintain than a
// full index across all 10,000,000 rows, and it exactly matches the query
// pattern that needs it.
func dbPGPartialIndexTest() testkit.Test {
	prompt := `The "orders" table holds 10,000,000 rows. Exactly 120,000 of
them have status='pending'. The application's single hottest query against
this table is "SELECT * FROM orders WHERE status = 'pending';", run almost
every time this table is queried at all.

Is a partial index (CREATE INDEX ... ON orders(...) WHERE status =
'pending') a good fit here, and why? Respond with only a JSON object:
{"partial_index_applicable":true|false,"reason":"<one of: small-frequently-queried-subset, subset-too-large, query-does-not-filter-on-column>"}`

	evaluator := eval.Mean(
		eval.JSONField("partial_index_applicable", true),
		eval.JSONField("reason", "small-frequently-queried-subset"),
	)

	return testkit.Test{
		ID:          "pg-partial-index",
		Category:    "databases",
		Subcategory: "postgres",
		Description: "Judge a partial index as a good fit for a 1.2%-selectivity, frequently-queried status value on a 10M-row table.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// dbPGListenNotifyTest: pick LISTEN/NOTIFY over polling for near-real-time
// notification of new rows without a tight polling loop.
//
// ground truth: LISTEN/NOTIFY lets a connected client block until the
// database itself pushes a notification on a channel (via NOTIFY, typically
// from a trigger on INSERT), giving near-real-time delivery with zero
// polling queries in between - unlike a polling loop, which either wastes
// queries when idle or adds latency when its interval is too coarse.
func dbPGListenNotifyTest() testkit.Test {
	prompt := `Background workers need near-real-time notification whenever a
new row is inserted into a "job_queue" table. The team wants to avoid a
tight polling loop that repeatedly queries the table (e.g. every second)
whether or not there is new work.

Which built-in Postgres mechanism lets workers avoid polling entirely,
being pushed a notification only when a new row actually arrives? Name the
mechanism.`

	evaluator := eval.Mean(
		eval.ContainsAny("LISTEN"),
		eval.ContainsAny("NOTIFY"),
	)

	return testkit.Test{
		ID:          "pg-listen-notify",
		Category:    "databases",
		Subcategory: "postgres",
		Description: "Pick LISTEN/NOTIFY over polling for near-real-time notification of new job_queue rows.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}
