package matrixbuilder

import "testing"

// leaf builds a comparison condition node.
func leaf(series, op string, value float64) *FilterNode {
	return &FilterNode{Series: series, Operator: op, Value: value}
}

func TestFilterNodeToQuery(t *testing.T) {
	tests := []struct {
		name string
		node *FilterNode
		want string // empty means ok=false (nothing to contribute)
	}{
		{
			"single leaf, no outer parens at root",
			&FilterNode{Combinator: "and", Children: []*FilterNode{leaf("rpm", ">", 3000)}},
			"rpm > 3000",
		},
		{
			"two anded at root",
			&FilterNode{Combinator: "and", Children: []*FilterNode{
				leaf("rpm", ">", 3000), leaf("load", ">", 50),
			}},
			"rpm > 3000 and load > 50",
		},
		{
			"nested group keeps its parens",
			&FilterNode{Combinator: "or", Children: []*FilterNode{
				{Combinator: "and", Children: []*FilterNode{leaf("rpm", ">", 3000), leaf("load", ">", 50)}},
				{Series: "ECMStat.ST_ActiveAirDem", Operator: "in", Values: []float64{10, 20}},
			}},
			"(rpm > 3000 and load > 50) or ECMStat.ST_ActiveAirDem in [10, 20]",
		},
		{
			"negated group at root",
			&FilterNode{Combinator: "and", Negate: true, Children: []*FilterNode{
				leaf("rpm", ">", 3000), leaf("load", ">", 50),
			}},
			"not (rpm > 3000 and load > 50)",
		},
		{
			"negated leaf",
			&FilterNode{Combinator: "and", Children: []*FilterNode{
				{Series: "rpm", Operator: ">", Value: 3000, Negate: true},
			}},
			"not (rpm > 3000)",
		},
		{
			"incomplete leaves are dropped",
			&FilterNode{Combinator: "and", Children: []*FilterNode{
				leaf("rpm", ">", 3000),
				{Series: "", Operator: ">", Value: 1},      // no series
				{Series: "x", Operator: "in", Values: nil}, // empty list
			}},
			"rpm > 3000",
		},
		{"empty group contributes nothing", &FilterNode{Combinator: "and"}, ""},
		{
			"group with only incomplete children contributes nothing",
			&FilterNode{Combinator: "and", Children: []*FilterNode{{Series: ""}}},
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.node.toQuery(true)
			if tt.want == "" {
				if ok {
					t.Fatalf("toQuery() = %q, want ok=false", got)
				}
				return
			}
			if !ok {
				t.Fatalf("toQuery() ok=false, want %q", tt.want)
			}
			if got != tt.want {
				t.Errorf("toQuery() = %q, want %q", got, tt.want)
			}
			// Whatever the builder emits must be a valid query.
			if _, err := compileQuery(got, resolverFrom(nil)); err != nil {
				t.Errorf("compileQuery(%q) error: %v", got, err)
			}
		})
	}
}

// The builder is just a structured query author, so a tree and its hand-written
// query equivalent must filter identically.
func TestFilterNodeMatchesQuery(t *testing.T) {
	series := map[string][]float64{
		"rpm":   {1000, 4000, 4000, 4000},
		"load":  {10, 60, 40, 60},
		"state": {10, 20, 30, 10},
	}
	resolve := resolverFrom(series)

	// (rpm > 3000 and load > 50) or state in [10, 20]
	tree := &FilterNode{Combinator: "or", Children: []*FilterNode{
		{Combinator: "and", Children: []*FilterNode{leaf("rpm", ">", 3000), leaf("load", ">", 50)}},
		{Series: "state", Operator: "in", Values: []float64{10, 20}},
	}}

	src, ok := tree.toQuery(true)
	if !ok {
		t.Fatal("toQuery() ok=false")
	}
	q, err := compileQuery(src, resolve)
	if err != nil {
		t.Fatalf("compileQuery(%q) error: %v", src, err)
	}
	want := [4]bool{true, true, false, true} // row0 state=10; row1 rpm&load; row3 state=10
	for i := 0; i < 4; i++ {
		if got := q.Eval(i); got != want[i] {
			t.Errorf("Eval(%d) = %v, want %v (query %q)", i, got, want[i], src)
		}
	}
}

func TestRulesToNodeMigration(t *testing.T) {
	rules := []Rule{
		{Series: "rpm", Operator: ">", Threshold: 3000},
		{Series: "load", Operator: "<=", Threshold: 80},
	}
	got, ok := rulesToNode(rules).toQuery(true)
	if !ok {
		t.Fatal("toQuery() ok=false")
	}
	want := "rpm > 3000 and load <= 80"
	if got != want {
		t.Errorf("migrated query = %q, want %q", got, want)
	}
}
