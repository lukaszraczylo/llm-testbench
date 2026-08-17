package tests

import (
	"context"
	"sort"
	"testing"
)

// TestDBSQLAggregateSumWant_GroundTruth independently re-derives the paid-
// orders total by listing the paid amounts by hand, not by calling
// dbSQLSumPaidAmount.
func TestDBSQLAggregateSumWant_GroundTruth(t *testing.T) {
	paidAmounts := []int{250, 100, 300, 400, 125} // ids 1, 2, 4, 6, 7
	want := 0
	for _, a := range paidAmounts {
		want += a
	}

	if want != 1175 {
		t.Fatalf("independently recomputed sum = %d, want 1175", want)
	}
	if dbSQLAggregateSumWant != want {
		t.Errorf("dbSQLAggregateSumWant = %d, independently recomputed = %d", dbSQLAggregateSumWant, want)
	}
}

func TestDBSQLAggregateSumTest_Eval(t *testing.T) {
	tc := dbSQLAggregateSumTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", "1175", 1},
		{"prose wrapped", "The sum is 1175.", 1},
		{"wrong: included a pending row", "1250", 0},
		{"wrong: excluded a paid row", "875", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

// TestDBSQLJoinCountsWant_GroundTruth independently re-derives both join
// counts from a hand-written per-customer_id tally, not by calling
// dbSQLInnerJoinCount/dbSQLLeftJoinCount.
func TestDBSQLJoinCountsWant_GroundTruth(t *testing.T) {
	orderCustomerIDs := []int{101, 102, 101, 103, 102, 104, 101, 103} // ids 1..8
	knownCustomers := map[int]bool{101: true, 102: true, 105: true}

	innerWant := 0
	perCustomerOrders := map[int]int{}
	for _, cid := range orderCustomerIDs {
		perCustomerOrders[cid]++
		if knownCustomers[cid] {
			innerWant++
		}
	}
	if innerWant != 5 {
		t.Fatalf("independently recomputed inner join count = %d, want 5", innerWant)
	}

	leftWant := 0
	for cid := range knownCustomers {
		if perCustomerOrders[cid] == 0 {
			leftWant++ // one NULL-order row
		} else {
			leftWant += perCustomerOrders[cid]
		}
	}
	if leftWant != 6 {
		t.Fatalf("independently recomputed left join count = %d, want 6", leftWant)
	}

	if dbSQLInnerJoinCountWant != innerWant {
		t.Errorf("dbSQLInnerJoinCountWant = %d, independently recomputed = %d", dbSQLInnerJoinCountWant, innerWant)
	}
	if dbSQLLeftJoinCountWant != leftWant {
		t.Errorf("dbSQLLeftJoinCountWant = %d, independently recomputed = %d", dbSQLLeftJoinCountWant, leftWant)
	}
}

func TestDBSQLJoinCountsTest_Eval(t *testing.T) {
	tc := dbSQLJoinCountsTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", `{"inner_join_count":5,"left_join_count":6}`, 1},
		{"correct fenced", "```json\n{\"inner_join_count\":5,\"left_join_count\":6}\n```", 1},
		{"wrong inner count", `{"inner_join_count":4,"left_join_count":6}`, 0.5},
		{"wrong left count: forgot the zero-match customer's NULL row", `{"inner_join_count":5,"left_join_count":5}`, 0.5},
		{"both wrong: treated inner and left as equal", `{"inner_join_count":8,"left_join_count":8}`, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

// TestDBSQLNullInWhereWant_GroundTruth independently re-derives the
// region <> 'EU' count by listing every row's region by hand, not by
// calling dbSQLCountRegionNotEU.
func TestDBSQLNullInWhereWant_GroundTruth(t *testing.T) {
	regions := []string{"EU", "US", "EU", "", "US", "APAC", "EU", "US"} // ids 1..8, "" = NULL
	want := 0
	for _, region := range regions {
		if region == "" {
			continue // NULL: <> 'EU' is UNKNOWN, excluded
		}
		if region != "EU" {
			want++
		}
	}

	if want != 4 {
		t.Fatalf("independently recomputed count = %d, want 4", want)
	}
	if dbSQLNullInWhereWant != want {
		t.Errorf("dbSQLNullInWhereWant = %d, independently recomputed = %d", dbSQLNullInWhereWant, want)
	}
}

func TestDBSQLNullInWhereTest_Eval(t *testing.T) {
	tc := dbSQLNullInWhereTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", "4", 1},
		{"prose wrapped", "The count is 4.", 1},
		{"wrong: forgot NULL exclusion (8 - 3 EU rows)", "5", 0},
		{"wrong: counted the NULL row as non-EU", "5", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

// TestDBSQLGroupByHavingWant_GroundTruth independently re-derives the
// GROUP BY + HAVING result from hand-written region totals, not by calling
// dbSQLGroupByRegionHaving.
func TestDBSQLGroupByHavingWant_GroundTruth(t *testing.T) {
	totals := map[string]int{
		"EU":   250 + 75 + 125,
		"US":   100 + 50 + 60,
		"APAC": 400,
	}
	if totals["EU"] != 450 || totals["US"] != 210 || totals["APAC"] != 400 {
		t.Fatalf("independently recomputed totals = %v, want EU=450 US=210 APAC=400", totals)
	}

	var kept []string
	for region, total := range totals {
		if total > 300 {
			kept = append(kept, region)
		}
	}
	sort.Strings(kept)

	if len(kept) != 2 || kept[0] != "APAC" || kept[1] != "EU" {
		t.Fatalf("independently recomputed kept regions = %v, want [APAC EU]", kept)
	}

	if len(dbSQLGroupByHavingWant) != 2 {
		t.Fatalf("dbSQLGroupByHavingWant has %d entries, want 2", len(dbSQLGroupByHavingWant))
	}
	if dbSQLGroupByHavingWant[0].Region != "APAC" || dbSQLGroupByHavingWant[0].Total != 400 {
		t.Errorf("dbSQLGroupByHavingWant[0] = %+v, want {APAC 400}", dbSQLGroupByHavingWant[0])
	}
	if dbSQLGroupByHavingWant[1].Region != "EU" || dbSQLGroupByHavingWant[1].Total != 450 {
		t.Errorf("dbSQLGroupByHavingWant[1] = %+v, want {EU 450}", dbSQLGroupByHavingWant[1])
	}
}

func TestDBSQLGroupByHavingTest_Eval(t *testing.T) {
	tc := dbSQLGroupByHavingTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", `[{"region":"APAC","total":400},{"region":"EU","total":450}]`, 1},
		{"correct fenced with prose", "Result:\n```json\n[{\"region\":\"APAC\",\"total\":400},{\"region\":\"EU\",\"total\":450}]\n```", 1},
		{"wrong order", `[{"region":"EU","total":450},{"region":"APAC","total":400}]`, 0.2},
		{"missing a group", `[{"region":"APAC","total":400}]`, 0.4},
		{"wrong total for one group", `[{"region":"APAC","total":401},{"region":"EU","total":450}]`, 0.8},
		{"forgot HAVING filter, includes US", `[{"region":"APAC","total":400},{"region":"EU","total":450},{"region":"US","total":210}]`, 0.8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

// TestDBSQLWindowRankWant_GroundTruth independently re-derives the top
// earner per department from hand-written salary lists, not by calling
// dbSQLTopEarnerByDepartment.
func TestDBSQLWindowRankWant_GroundTruth(t *testing.T) {
	eng := map[string]int{"Alice": 140, "Bob": 120, "Erin": 150}
	sales := map[string]int{"Carol": 110, "Dave": 130, "Frank": 90}

	topOf := func(salaries map[string]int) string {
		best, bestSalary := "", -1
		for name, salary := range salaries {
			if salary > bestSalary {
				best, bestSalary = name, salary
			}
		}
		return best
	}

	if got := topOf(eng); got != "Erin" {
		t.Fatalf("independently recomputed Eng top earner = %q, want Erin", got)
	}
	if got := topOf(sales); got != "Dave" {
		t.Fatalf("independently recomputed Sales top earner = %q, want Dave", got)
	}

	if dbSQLWindowRankWant["Eng"] != "Erin" {
		t.Errorf(`dbSQLWindowRankWant["Eng"] = %q, want "Erin"`, dbSQLWindowRankWant["Eng"])
	}
	if dbSQLWindowRankWant["Sales"] != "Dave" {
		t.Errorf(`dbSQLWindowRankWant["Sales"] = %q, want "Dave"`, dbSQLWindowRankWant["Sales"])
	}
}

func TestDBSQLWindowRankTest_Eval(t *testing.T) {
	tc := dbSQLWindowRankTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", `{"Eng":"Erin","Sales":"Dave"}`, 1},
		{"correct fenced", "```json\n{\"Eng\":\"Erin\",\"Sales\":\"Dave\"}\n```", 1},
		{"wrong Eng", `{"Eng":"Alice","Sales":"Dave"}`, 0.5},
		{"wrong Sales", `{"Eng":"Erin","Sales":"Carol"}`, 0.5},
		{"both wrong", `{"Eng":"Bob","Sales":"Frank"}`, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestDBSQLCompositeIndexOrderTest_Eval(t *testing.T) {
	tc := dbSQLCompositeIndexOrderTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", `["user_id", "created_at"]`, 1},
		{"correct fenced", "```json\n[\"user_id\", \"created_at\"]\n```", 1},
		{"reversed order", `["created_at", "user_id"]`, 0},
		{"missing a column", `["user_id"]`, 0},
		{"wrong column entirely", `["user_id", "ip"]`, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestDBSQLKeysetPaginationTest_Eval(t *testing.T) {
	tc := dbSQLKeysetPaginationTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"bare correct", "keyset", 1},
		{"quoted correct", `"keyset"`, 1},
		{"different case", "Keyset", 1},
		{"trailing period", "keyset.", 1},
		{"wrong", "offset", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestDBSQLCoveringIndexTest_Eval(t *testing.T) {
	tc := dbSQLCoveringIndexTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"bare correct", "yes", 1},
		{"quoted correct", `"yes"`, 1},
		{"trailing period", "Yes.", 1},
		{"wrong", "no", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestDBSQLEquivalentRewriteTest_Eval(t *testing.T) {
	tc := dbSQLEquivalentRewriteTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", `{"A":true,"B":false,"C":true}`, 1},
		{"correct fenced with prose", "Here's my analysis:\n```json\n{\"A\":true,\"B\":false,\"C\":true}\n```", 1},
		{"wrongly accepts B (impossible AND)", `{"A":true,"B":true,"C":true}`, 2.0 / 3.0},
		{"wrongly rejects C (forgets closed value set)", `{"A":true,"B":false,"C":false}`, 2.0 / 3.0},
		{"everything wrong", `{"A":false,"B":true,"C":false}`, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestDBSQLOrderByIndexServingTest_Eval(t *testing.T) {
	tc := dbSQLOrderByIndexServingTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", `{"avoids_sort_step":true,"reason":"index-matches-filter-and-order"}`, 1},
		{"correct fenced", "```json\n{\"avoids_sort_step\":true,\"reason\":\"index-matches-filter-and-order\"}\n```", 1},
		{"wrongly says a sort step is needed", `{"avoids_sort_step":false,"reason":"index-matches-filter-and-order"}`, 0.5},
		{"wrong reason", `{"avoids_sort_step":true,"reason":"order-by-column-not-leading"}`, 0.5},
		{"both wrong", `{"avoids_sort_step":false,"reason":"sort-direction-mismatch"}`, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}
