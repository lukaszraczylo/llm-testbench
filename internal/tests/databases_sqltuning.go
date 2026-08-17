package tests

import (
	"sort"

	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

// registerDBSQLTuningTests registers every databases/sql-tuning test.
func registerDBSQLTuningTests(r *testkit.Registry) {
	r.Register(dbSQLAggregateSumTest())
	r.Register(dbSQLJoinCountsTest())
	r.Register(dbSQLNullInWhereTest())
	r.Register(dbSQLGroupByHavingTest())
	r.Register(dbSQLWindowRankTest())
	r.Register(dbSQLCompositeIndexSkipColumnTest())
	r.Register(dbSQLKeysetPaginationTest())
	r.Register(dbSQLCoveringIndexTest())
	r.Register(dbSQLEquivalentRewriteTest())
	r.Register(dbSQLOrderByIndexServingTest())
}

// dbSQLOrder is one row of the shared inline "orders" table fixture used by
// several sql-tuning tests. Region is "" to represent SQL NULL (order id 4
// has no region on file).
type dbSQLOrder struct {
	Region     string
	Status     string
	ID         int
	CustomerID int
	Amount     int
}

// dbSQLOrders is the shared inline "orders" table fixture. It is printed
// verbatim (see dbSQLOrdersTableText) into several sql-tuning prompts, and
// every one of those tests' expected answers is derived by calling a query-
// logic helper on this exact slice - never hardcoded independently of it.
var dbSQLOrders = []dbSQLOrder{
	{ID: 1, CustomerID: 101, Region: "EU", Amount: 250, Status: "paid"},
	{ID: 2, CustomerID: 102, Region: "US", Amount: 100, Status: "paid"},
	{ID: 3, CustomerID: 101, Region: "EU", Amount: 75, Status: "pending"},
	{ID: 4, CustomerID: 103, Region: "", Amount: 300, Status: "paid"},
	{ID: 5, CustomerID: 102, Region: "US", Amount: 50, Status: "refunded"},
	{ID: 6, CustomerID: 104, Region: "APAC", Amount: 400, Status: "paid"},
	{ID: 7, CustomerID: 101, Region: "EU", Amount: 125, Status: "paid"},
	{ID: 8, CustomerID: 103, Region: "US", Amount: 60, Status: "pending"},
}

// dbSQLOrdersTableText is dbSQLOrders rendered as a plain-text table for
// the prompt. Kept in sync with dbSQLOrders by hand; databases_sqltuning_
// test.go's ground-truth tests operate on dbSQLOrders directly, so a
// mismatch here would only affect prompt readability, never the pinned
// expected answers.
const dbSQLOrdersTableText = `id | customer_id | region | amount | status
1  | 101         | EU     | 250    | paid
2  | 102         | US     | 100    | paid
3  | 101         | EU     | 75     | pending
4  | 103         | NULL   | 300    | paid
5  | 102         | US     | 50     | refunded
6  | 104         | APAC   | 400    | paid
7  | 101         | EU     | 125    | paid
8  | 103         | US     | 60     | pending`

// dbSQLSumPaidAmount sums Amount over every row whose Status is "paid" -
// the query-logic equivalent of "SELECT SUM(amount) FROM orders WHERE
// status = 'paid'".
func dbSQLSumPaidAmount(rows []dbSQLOrder) int {
	total := 0
	for _, r := range rows {
		if r.Status == "paid" {
			total += r.Amount
		}
	}
	return total
}

// dbSQLAggregateSumWant is derived by calling dbSQLSumPaidAmount, not
// hardcoded.
//
// ground truth: paid rows are id1(250), id2(100), id4(300), id6(400),
// id7(125): 250+100+300+400+125 = 1175. databases_sqltuning_test.go
// recomputes this independently with a hand-written loop over the same
// dbSQLOrders fixture.
var dbSQLAggregateSumWant = dbSQLSumPaidAmount(dbSQLOrders)

// dbSQLAggregateSumTest: compute SUM(amount) WHERE status = 'paid' over an
// inline 8-row orders table.
func dbSQLAggregateSumTest() testkit.Test {
	prompt := `Here is the "orders" table:

` + "```\n" + dbSQLOrdersTableText + "\n```" + `

What does "SELECT SUM(amount) FROM orders WHERE status = 'paid';" return?
Respond with only the number.`

	return testkit.Test{
		ID:          "sql-aggregate-sum",
		Category:    "databases",
		Subcategory: "sql-tuning",
		Description: "Compute SUM(amount) WHERE status = 'paid' over an inline 8-row orders table.",
		Prompt:      prompt,
		Eval:        eval.Numeric(eval.ExtractLastNumber[int], dbSQLAggregateSumWant, 0),
	}
}

// dbSQLCustomer is one row of the shared inline "customers" table fixture
// used by dbSQLJoinCountsTest. It intentionally does not cover every
// customer_id present in dbSQLOrders (103 and 104 are absent), and
// includes one customer (105) with no matching orders at all, so INNER
// JOIN and LEFT JOIN produce different row counts.
type dbSQLCustomer struct {
	Name string
	ID   int
}

// dbSQLCustomers is the shared inline "customers" table fixture, printed
// verbatim into dbSQLJoinCountsTest's prompt.
var dbSQLCustomers = []dbSQLCustomer{
	{ID: 101, Name: "Alice"},
	{ID: 102, Name: "Bob"},
	{ID: 105, Name: "Eve"},
}

// dbSQLCustomersTableText is dbSQLCustomers rendered as a plain-text table
// for the prompt.
const dbSQLCustomersTableText = `id  | name
101 | Alice
102 | Bob
105 | Eve`

// dbSQLInnerJoinCount counts the result rows of "orders o INNER JOIN
// customers c ON o.customer_id = c.id": one row per order whose
// customer_id matches some customer in customers, zero rows contributed by
// a customer or order with no match on the other side.
func dbSQLInnerJoinCount(orders []dbSQLOrder, customers []dbSQLCustomer) int {
	knownCustomer := make(map[int]bool, len(customers))
	for _, c := range customers {
		knownCustomer[c.ID] = true
	}
	count := 0
	for _, o := range orders {
		if knownCustomer[o.CustomerID] {
			count++
		}
	}
	return count
}

// dbSQLLeftJoinCount counts the result rows of "customers c LEFT JOIN
// orders o ON o.customer_id = c.id": one row per matching order, or
// exactly one row (with NULL order columns) for a customer with zero
// matching orders.
func dbSQLLeftJoinCount(customers []dbSQLCustomer, orders []dbSQLOrder) int {
	count := 0
	for _, c := range customers {
		matches := 0
		for _, o := range orders {
			if o.CustomerID == c.ID {
				matches++
			}
		}
		if matches == 0 {
			count++
		} else {
			count += matches
		}
	}
	return count
}

// dbSQLInnerJoinCountWant and dbSQLLeftJoinCountWant are derived by calling
// dbSQLInnerJoinCount/dbSQLLeftJoinCount, not hardcoded.
//
// ground truth: orders' customer_id values are 101(x3: id1,3,7), 102(x2:
// id2,5), 103(x2: id4,8), 104(x1: id6). customers only holds ids
// {101,102,105}. INNER JOIN keeps only rows whose customer_id is in that
// set: 3+2 = 5 (customer 105 contributes nothing; customer_id 103/104
// orders are dropped). LEFT JOIN, driven from customers, keeps every
// customer: 101 contributes 3, 102 contributes 2, 105 contributes exactly
// 1 row (NULL order columns, since it matches zero orders): 3+2+1 = 6.
// databases_sqltuning_test.go recomputes both independently with a
// hand-written loop.
var (
	dbSQLInnerJoinCountWant = dbSQLInnerJoinCount(dbSQLOrders, dbSQLCustomers)
	dbSQLLeftJoinCountWant  = dbSQLLeftJoinCount(dbSQLCustomers, dbSQLOrders)
)

// dbSQLJoinCountsTest: compute the row counts of an INNER JOIN versus a
// LEFT JOIN between the same inline orders and customers tables.
func dbSQLJoinCountsTest() testkit.Test {
	prompt := `Here are two tables:

orders:
` + "```\n" + dbSQLOrdersTableText + "\n```" + `

customers:
` + "```\n" + dbSQLCustomersTableText + "\n```" + `

How many rows does each of these two queries return?

query_inner: SELECT o.id FROM orders o INNER JOIN customers c ON o.customer_id = c.id;
query_left: SELECT c.name, o.id FROM customers c LEFT JOIN orders o ON o.customer_id = c.id;

Respond with only a JSON object:
{"inner_join_count":<number>,"left_join_count":<number>}`

	evaluator := eval.Mean(
		eval.JSONField("inner_join_count", dbSQLInnerJoinCountWant),
		eval.JSONField("left_join_count", dbSQLLeftJoinCountWant),
	)

	return testkit.Test{
		ID:          "sql-join-counts",
		Category:    "databases",
		Subcategory: "sql-tuning",
		Description: "Compute INNER JOIN (5) versus LEFT JOIN (6) row counts between an inline orders and customers table.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// dbSQLCountRegionNotEU counts rows where "region <> 'EU'" evaluates to
// SQL's three-valued TRUE. A row whose Region is SQL NULL (Region == "")
// makes that comparison evaluate to UNKNOWN, not TRUE, so it is excluded -
// not counted as matching EU and not counted as matching the negation
// either.
func dbSQLCountRegionNotEU(rows []dbSQLOrder) int {
	count := 0
	for _, r := range rows {
		if r.Region == "" {
			continue // NULL: comparison is UNKNOWN, row excluded either way
		}
		if r.Region != "EU" {
			count++
		}
	}
	return count
}

// dbSQLNullInWhereWant is derived by calling dbSQLCountRegionNotEU, not
// hardcoded.
//
// ground truth: region values are EU(id1,3,7), US(id2,5,8), APAC(id6),
// NULL(id4). "region <> 'EU'" is TRUE for the 4 US/APAC rows, FALSE for
// the 3 EU rows, and UNKNOWN (excluded) for the 1 NULL row - the trap
// being that a naive 8-3=5 count forgets NULL is excluded from BOTH sides
// of the comparison. databases_sqltuning_test.go recomputes this
// independently with a hand-written loop.
var dbSQLNullInWhereWant = dbSQLCountRegionNotEU(dbSQLOrders)

// dbSQLNullInWhereTest: compute the row count of a WHERE clause negating a
// column that contains a NULL, testing whether the model accounts for
// SQL's three-valued logic (NULL never satisfies <>, so it is excluded
// from both the match and its negation).
func dbSQLNullInWhereTest() testkit.Test {
	prompt := `Here is the "orders" table (region is NULL for order id 4):

` + "```\n" + dbSQLOrdersTableText + "\n```" + `

What does "SELECT COUNT(*) FROM orders WHERE region <> 'EU';" return?
Remember SQL's three-valued logic: a comparison against NULL evaluates to
UNKNOWN, not TRUE or FALSE, so a NULL row satisfies neither
"region = 'EU'" nor "region <> 'EU'". Respond with only the number.`

	return testkit.Test{
		ID:          "sql-null-in-where",
		Category:    "databases",
		Subcategory: "sql-tuning",
		Description: "Compute COUNT(*) WHERE region <> 'EU' over an inline table with one NULL region, testing the NULL-exclusion trap.",
		Prompt:      prompt,
		Eval:        eval.Numeric(eval.ExtractLastNumber[int], dbSQLNullInWhereWant, 0),
	}
}

// dbSQLRegionTotal is one row of a GROUP BY region result: the region name
// and its summed amount.
type dbSQLRegionTotal struct {
	Region string
	Total  int
}

// dbSQLGroupByRegionHaving computes SUM(amount) per non-NULL region, keeps
// only groups whose total exceeds minTotal, and returns them sorted by
// region ascending - the query-logic equivalent of "... WHERE region IS
// NOT NULL GROUP BY region HAVING SUM(amount) > minTotal ORDER BY region".
func dbSQLGroupByRegionHaving(rows []dbSQLOrder, minTotal int) []dbSQLRegionTotal {
	totals := make(map[string]int)
	var regions []string
	for _, r := range rows {
		if r.Region == "" {
			continue
		}
		if _, seen := totals[r.Region]; !seen {
			regions = append(regions, r.Region)
		}
		totals[r.Region] += r.Amount
	}
	sort.Strings(regions)

	var out []dbSQLRegionTotal
	for _, region := range regions {
		if totals[region] > minTotal {
			out = append(out, dbSQLRegionTotal{Region: region, Total: totals[region]})
		}
	}
	return out
}

// dbSQLGroupByHavingWant is derived by calling dbSQLGroupByRegionHaving,
// not hardcoded.
//
// ground truth: non-NULL region totals are EU = 250+75+125 = 450, US =
// 100+50+60 = 210, APAC = 400. HAVING SUM(amount) > 300 keeps EU (450) and
// APAC (400), drops US (210). Sorted by region ascending: APAC before EU.
// databases_sqltuning_test.go recomputes this independently with a
// hand-written map + sort.
var dbSQLGroupByHavingWant = dbSQLGroupByRegionHaving(dbSQLOrders, 300)

// dbSQLGroupByHavingTest: compute a GROUP BY + HAVING result (region
// totals over 300) over the inline orders table, as an ordered JSON array.
func dbSQLGroupByHavingTest() testkit.Test {
	prompt := `Here is the "orders" table:

` + "```\n" + dbSQLOrdersTableText + "\n```" + `

What does this query return?

SELECT region, SUM(amount) AS total
FROM orders
WHERE region IS NOT NULL
GROUP BY region
HAVING SUM(amount) > 300
ORDER BY region;

Respond with only a JSON array of objects, ordered exactly as the query's
ORDER BY would return them: [{"region":"...","total":<number>}, ...]`

	// D9: weight the array-length check 2x each individual field check, so
	// a response that leaks an extra group through (e.g. forgetting to
	// apply the HAVING filter) cannot stay close to full credit just by
	// getting the two checked elements right - the structural correctness
	// of "exactly the right number of groups" matters more than any one
	// field.
	evaluator := eval.All(
		eval.W(dbJSONArrayLength(len(dbSQLGroupByHavingWant)), 2),
		eval.W(eval.JSONField("[0].region", dbSQLGroupByHavingWant[0].Region), 1),
		eval.W(eval.JSONField("[0].total", dbSQLGroupByHavingWant[0].Total), 1),
		eval.W(eval.JSONField("[1].region", dbSQLGroupByHavingWant[1].Region), 1),
		eval.W(eval.JSONField("[1].total", dbSQLGroupByHavingWant[1].Total), 1),
	)

	return testkit.Test{
		ID:          "sql-groupby-having",
		Category:    "databases",
		Subcategory: "sql-tuning",
		Description: "Compute a GROUP BY region + HAVING SUM(amount) > 300 result (APAC, EU) over the inline orders table, as an ordered JSON array.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// dbSQLEmployee is one row of the shared inline "employees" table fixture
// used by dbSQLWindowRankTest.
type dbSQLEmployee struct {
	Name       string
	Department string
	Salary     int
}

// dbSQLEmployees is the shared inline "employees" table fixture, printed
// verbatim into dbSQLWindowRankTest's prompt. No department has a salary
// tie, so RANK() ... = 1 identifies exactly one employee per department.
var dbSQLEmployees = []dbSQLEmployee{
	{Name: "Alice", Department: "Eng", Salary: 140},
	{Name: "Bob", Department: "Eng", Salary: 120},
	{Name: "Carol", Department: "Sales", Salary: 110},
	{Name: "Dave", Department: "Sales", Salary: 130},
	{Name: "Erin", Department: "Eng", Salary: 150},
	{Name: "Frank", Department: "Sales", Salary: 90},
}

// dbSQLEmployeesTableText is dbSQLEmployees rendered as a plain-text table
// for the prompt.
const dbSQLEmployeesTableText = `name  | department | salary
Alice | Eng        | 140
Bob   | Eng        | 120
Carol | Sales      | 110
Dave  | Sales      | 130
Erin  | Eng        | 150
Frank | Sales      | 90`

// dbSQLTopEarnerByDepartment returns, for each department present in rows,
// the name of the employee with the highest salary in that department -
// the query-logic equivalent of "RANK() OVER (PARTITION BY department
// ORDER BY salary DESC) = 1", assuming no in-department salary tie.
func dbSQLTopEarnerByDepartment(rows []dbSQLEmployee) map[string]string {
	best := make(map[string]dbSQLEmployee, len(rows))
	for _, e := range rows {
		cur, ok := best[e.Department]
		if !ok || e.Salary > cur.Salary {
			best[e.Department] = e
		}
	}
	out := make(map[string]string, len(best))
	for dept, e := range best {
		out[dept] = e.Name
	}
	return out
}

// dbSQLWindowRankWant is derived by calling dbSQLTopEarnerByDepartment, not
// hardcoded.
//
// ground truth: Eng salaries are Alice=140, Bob=120, Erin=150 - Erin is
// highest. Sales salaries are Carol=110, Dave=130, Frank=90 - Dave is
// highest. databases_sqltuning_test.go recomputes this independently with
// a hand-written loop.
var dbSQLWindowRankWant = dbSQLTopEarnerByDepartment(dbSQLEmployees)

// dbSQLWindowRankTest: identify the rank-1 (highest-salary) employee per
// department from a RANK() OVER (PARTITION BY ...) window function applied
// to an inline employees table.
func dbSQLWindowRankTest() testkit.Test {
	prompt := `Here is the "employees" table:

` + "```\n" + dbSQLEmployeesTableText + "\n```" + `

For "SELECT department, name, RANK() OVER (PARTITION BY department ORDER
BY salary DESC) AS rnk FROM employees;", which employee's name has rnk = 1
in each department? Respond with only a JSON object:
{"Eng":"<name>","Sales":"<name>"}`

	evaluator := eval.Mean(
		eval.JSONField("Eng", dbSQLWindowRankWant["Eng"]),
		eval.JSONField("Sales", dbSQLWindowRankWant["Sales"]),
	)

	return testkit.Test{
		ID:          "sql-window-rank",
		Category:    "databases",
		Subcategory: "sql-tuning",
		Description: "Identify the RANK()=1 (highest-salary) employee per department from an inline 6-row employees table.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// dbSQLCompositeIndexSkipSchema is the inline schema + index + query for
// dbSQLCompositeIndexSkipColumnTest.
//
// D10: this test replaces the former sql-composite-index-order, which
// duplicated pg-index-choice (databases_postgres.go) almost exactly -
// same "pick equality-then-range column order" question shape, just a
// different table/column naming. This test instead covers a genuinely
// distinct btree composite-index rule: a query that skips the middle
// column of a 3-column index entirely.
const dbSQLCompositeIndexSkipSchema = `Table: events(id bigserial, tenant_id bigint, event_type text, created_at timestamptz)

Index: CREATE INDEX idx_events_tenant_type_created ON events(tenant_id, event_type, created_at);

Query:
SELECT * FROM events
WHERE tenant_id = 42 AND created_at > '2026-01-01';`

// dbSQLCompositeIndexSkipColumnTest: identify which column of a 3-column
// composite index actually serves as an index seek (Index Cond) versus
// which column can only be applied as a post-index Filter, when the
// query's WHERE clause skips the index's middle column entirely.
//
// ground truth: the composite btree index (tenant_id, event_type,
// created_at) is physically sorted first by tenant_id, then by event_type
// WITHIN each tenant_id, then by created_at WITHIN each (tenant_id,
// event_type) pair. The query constrains tenant_id (equality) and
// created_at (range) but never constrains event_type - the index's middle
// column - at all. Because created_at is only sorted within each
// event_type sub-group (not globally within a tenant_id), Postgres cannot
// use created_at as part of the index seek when event_type is
// unconstrained: it can only narrow the seek to the tenant_id=42 range
// (Index Cond: tenant_id = 42) and must apply created_at > '2026-01-01' as
// a Filter evaluated against every row in that range, not as part of the
// seek itself. This is the standard btree composite-index rule that only
// a contiguous, leading, query-constrained prefix of index columns can be
// used for seeking - skipping an unconstrained column strands every
// column after it as filter-only.
func dbSQLCompositeIndexSkipColumnTest() testkit.Test {
	prompt := `Here is a table schema, a composite index, and a query that
runs frequently:

` + "```\n" + dbSQLCompositeIndexSkipSchema + "\n```" + `

Note the query does NOT filter on event_type, the index's middle column.
A btree index can only be used as a seek (an Index Cond) for a contiguous
prefix of its columns that the query actually constrains, starting from
the leading column; it cannot skip an unconstrained column to seek on a
later one, because index entries are only sorted by that later column
WITHIN each value of the skipped column.

Which single column does this index serve as an efficient Index Cond
(seek)? Which single column can only be applied as a Filter after the
index narrows to the matching range (not as part of the index seek
itself)? Respond with only a JSON object:
{"index_cond_column":"<column>","filter_only_column":"<column>"}`

	evaluator := eval.Mean(
		eval.JSONField("index_cond_column", "tenant_id"),
		eval.JSONField("filter_only_column", "created_at"),
	)

	return testkit.Test{
		ID:          "sql-composite-index-skip-column",
		Category:    "databases",
		Subcategory: "sql-tuning",
		Description: "Identify that skipping a 3-column composite index's middle column strands the query's range filter as a post-index Filter, not a seek.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// dbSQLKeysetPaginationTest: pick keyset (seek) pagination over OFFSET
// pagination for a deep-paging scenario over a huge table.
//
// ground truth: "LIMIT 100 OFFSET 20000000" forces the database to walk
// and discard 20,000,000 rows before returning the next 100, so its cost
// grows with how deep the user pages. Keyset/seek pagination
// ("WHERE id > :last_seen_id ORDER BY id LIMIT 100") instead jumps
// straight to the right position via an index seek, so its latency stays
// roughly constant no matter how deep the user has paged.
func dbSQLKeysetPaginationTest() testkit.Test {
	prompt := `A table has 50,000,000 rows. An API paginates results ordered
by primary key id ascending, 100 rows per page. A user navigates deep into
the results - page 200,000, equivalent to OFFSET 20,000,000.

Which pagination technique keeps query latency roughly constant regardless
of how deep the user pages: OFFSET-based pagination
("LIMIT 100 OFFSET 20000000"), or keyset/seek pagination
("WHERE id > :last_seen_id ORDER BY id LIMIT 100")? Respond with only one
word: offset or keyset.`

	return testkit.Test{
		ID:          "sql-keyset-pagination",
		Category:    "databases",
		Subcategory: "sql-tuning",
		Description: "Pick keyset/seek pagination over OFFSET pagination for constant latency 20M rows deep into a 50M-row table.",
		Prompt:      prompt,
		Eval:        eval.ExactToken("keyset"),
	}
}

// dbSQLCoveringIndexTest: confirm that a composite index containing every
// column a query needs allows an index-only scan, with no heap visit.
//
// ground truth: the query only reads customer_id and status, and both are
// present in the index (customer_id, status) itself. Since every column
// the query needs is already inside the index entry, Postgres can answer
// the query by reading the index alone - an index-only scan - without
// visiting the underlying table (the heap) for any row, provided the
// visibility map allows it (as the prompt states).
func dbSQLCoveringIndexTest() testkit.Test {
	prompt := `Query: SELECT customer_id, status FROM orders WHERE customer_id = 42;

An index exists on orders(customer_id, status) - both columns this query
reads are present in the index itself. Assuming the visibility map allows
an index-only scan, can Postgres answer this query using only the index,
without visiting the underlying table (the heap) for any row? Respond with
only "yes" or "no".`

	return testkit.Test{
		ID:          "sql-covering-index",
		Category:    "databases",
		Subcategory: "sql-tuning",
		Description: "Confirm an index containing every column a query needs (customer_id, status) enables an index-only scan with no heap visit.",
		Prompt:      prompt,
		Eval:        eval.ExactToken("yes"),
	}
}

// dbSQLEquivalentRewriteQueries is the inline original query and 3
// rewrite candidates for dbSQLEquivalentRewriteTest.
const dbSQLEquivalentRewriteQueries = `Original: SELECT * FROM orders WHERE status = 'paid' OR status = 'pending';

Candidate A: SELECT * FROM orders WHERE status IN ('paid', 'pending');
Candidate B: SELECT * FROM orders WHERE status = 'paid' AND status = 'pending';
Candidate C: SELECT * FROM orders WHERE status != 'refunded';`

// dbSQLEquivalentRewriteTest: judge which of 3 rewrite candidates are
// semantically equivalent to an inline original query, given a stated
// closed set of possible column values.
//
// ground truth: status only ever takes one of exactly 3 values in this
// table: 'paid', 'pending', 'refunded' (stated in the prompt so C's
// equivalence is well-defined). Candidate A: "status IN (x, y)" is
// standard SQL sugar for "status = x OR status = y" - always equivalent
// to the original. Candidate B: a single column cannot equal two
// different literal values in the same row, so "status = 'paid' AND
// status = 'pending'" is always FALSE - never equivalent to a query that
// is often TRUE. Candidate C: since status only ever takes one of exactly
// 3 values, "status != 'refunded'" is TRUE exactly when status is 'paid'
// or 'pending' - equivalent to the original given that closed value set
// (it would NOT be equivalent if a 4th status value existed).
func dbSQLEquivalentRewriteTest() testkit.Test {
	prompt := `The "status" column in this table only ever holds one of
exactly 3 values: 'paid', 'pending', or 'refunded'.

` + dbSQLEquivalentRewriteQueries + `

Which of Candidate A, B, C are semantically equivalent to the Original
query (return exactly the same rows for any data satisfying that 3-value
constraint)? Respond with only a JSON object:
{"A":true|false,"B":true|false,"C":true|false}`

	evaluator := eval.Mean(
		eval.JSONField("A", true),
		eval.JSONField("B", false),
		eval.JSONField("C", true),
	)

	return testkit.Test{
		ID:          "sql-equivalent-rewrite",
		Category:    "databases",
		Subcategory: "sql-tuning",
		Description: "Judge 3 rewrite candidates (IN-sugar equivalent, impossible-AND non-equivalent, closed-set-negation equivalent) against an inline original OR query.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// dbSQLOrderByIndexServingSchema is the inline index definition + query
// for dbSQLOrderByIndexServingTest.
const dbSQLOrderByIndexServingSchema = `Index: CREATE INDEX idx_orders_status_created_at ON orders(status, created_at DESC);

Query:
SELECT * FROM orders
WHERE status = 'paid'
ORDER BY created_at DESC
LIMIT 20;`

// dbSQLOrderByIndexServingTest: judge whether an inline composite index
// can serve both a query's WHERE filter and its ORDER BY without a
// separate sort step.
//
// ground truth: the index leads with status (an exact equality match in
// the query), and its second column, created_at DESC, exactly matches
// both the query's remaining filter role and its ORDER BY created_at DESC
// direction. Postgres can therefore walk the matching status='paid'
// portion of the index in exactly the order the query needs, serving the
// ORDER BY directly from the index with no separate sort step.
func dbSQLOrderByIndexServingTest() testkit.Test {
	prompt := `Here is an index and a query:

` + "```\n" + dbSQLOrderByIndexServingSchema + "\n```" + `

Can Postgres use this index to satisfy both the WHERE filter and the
ORDER BY without a separate sort step? Respond with only a JSON object:
{"avoids_sort_step":true|false,"reason":"<one of: index-matches-filter-and-order, order-by-column-not-leading, sort-direction-mismatch>"}`

	evaluator := eval.Mean(
		eval.JSONField("avoids_sort_step", true),
		eval.JSONField("reason", "index-matches-filter-and-order"),
	)

	return testkit.Test{
		ID:          "sql-orderby-index-serving",
		Category:    "databases",
		Subcategory: "sql-tuning",
		Description: "Judge that an index on (status, created_at DESC) serves both an equality WHERE and an ORDER BY created_at DESC with no extra sort step.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}
