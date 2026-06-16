package matrixbuilder

import (
	"math"
	"testing"
)

// resolverFrom builds a query resolver over an in-memory series map, mirroring
// MatrixBuilder.resolve (missing series or a NaN reading fail with ok=false).
func resolverFrom(series map[string][]float64) func(string, int) (float64, bool) {
	return func(name string, i int) (float64, bool) {
		data, ok := series[name]
		if !ok || i < 0 || i >= len(data) {
			return 0, false
		}
		v := data[i]
		if math.IsNaN(v) {
			return 0, false
		}
		return v, true
	}
}

func TestCompileQueryEval(t *testing.T) {
	// One sample per row; index i selects the row under test.
	series := map[string][]float64{
		"rpm":               {1000, 4000, 4000, 4000, math.NaN()},
		"load":              {10, 60, 40, 60, 60},
		"boost":             {0.5, 1.2, 1.205, 0.8, 1.2},
		"ActualIn.n_Engine": {1000, 4000, 4000, 4000, 4000},
	}
	resolve := resolverFrom(series)

	tests := []struct {
		name  string
		query string
		want  [5]bool
	}{
		{"simple gt", "rpm > 3000", [5]bool{false, true, true, true, false}},
		{"and", "rpm > 3000 and load > 50", [5]bool{false, true, false, true, false}},
		// Row 4 has rpm=NaN (so rpm<2000 fails) but load=60>50, so the OR holds.
		{"or", "rpm < 2000 or load > 50", [5]bool{true, true, false, true, true}},
		{
			// Grouping changes the result vs. default precedence (&& binds tighter).
			"grouping", "(rpm == 4000 and load == 40) or rpm == 1000",
			[5]bool{true, false, true, false, false},
		},
		{
			"precedence no parens", "rpm == 1000 or rpm == 4000 and load == 40",
			[5]bool{true, false, true, false, false},
		},
		{"if prefix stripped", "if rpm > 3000", [5]bool{false, true, true, true, false}},
		{"not parenthesized", "not (rpm > 3000)", [5]bool{true, false, false, false, true}},
		{"approx hit", "boost ~ 1.2", [5]bool{false, true, true, false, true}},
		{"ne", "load != 60", [5]bool{true, false, true, false, false}},
		{"dotted series", "ActualIn.n_Engine >= 4000", [5]bool{false, true, true, true, true}},
		// load = {10,60,40,60,60}; only the 40 and the three 60s are members.
		{"in set", "load in [40, 60]", [5]bool{false, true, true, true, true}},
		{"in single", "load in [10]", [5]bool{true, false, false, false, false}},
		// Dotted name on the left, float members, combined with another clause.
		{"in dotted and", "ActualIn.n_Engine in [1000, 4000] and load < 50", [5]bool{true, false, true, false, false}},
		// not wrapping an in-test; row 4's rpm=NaN value fails the test, so not-> true.
		{"not in", "not (rpm in [1000, 4000])", [5]bool{false, false, false, false, true}},
		{"series vs series", "rpm > load", [5]bool{true, true, true, true, false}},
		// rpm/100 = {10,40,40,40,NaN}; load = {10,60,40,60,60}. Row 2 ties (40>40
		// false) and row 4's NaN rpm makes the value fail.
		{"arithmetic rhs", "load > rpm / 100", [5]bool{false, true, false, true, false}},
		{"nan fails", "rpm == 4000", [5]bool{false, true, true, true, false}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := compileQuery(tt.query, resolve)
			if err != nil {
				t.Fatalf("compileQuery(%q) error: %v", tt.query, err)
			}
			for i := 0; i < 5; i++ {
				if got := q.Eval(i); got != tt.want[i] {
					t.Errorf("Eval(%d) = %v, want %v", i, got, tt.want[i])
				}
			}
		})
	}
}

func TestCompileQuerySeries(t *testing.T) {
	q, err := compileQuery("rpm > 3000 and ActualIn.n_Engine < 6000 or rpm == 0", resolverFrom(nil))
	if err != nil {
		t.Fatalf("compileQuery error: %v", err)
	}
	got := q.Series()
	want := []string{"rpm", "ActualIn.n_Engine"} // first-seen order, deduped
	if len(got) != len(want) {
		t.Fatalf("Series() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Series()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestCompileQueryErrors(t *testing.T) {
	resolve := resolverFrom(nil)
	for _, query := range []string{
		"",               // empty
		"rpm >",          // dangling operator
		"rpm",            // not a condition
		"rpm + 10",       // value, not a condition
		"rpm > 3000 and", // dangling and
		"(rpm > 3000",    // unbalanced paren
		"rpm in []",      // empty membership list
	} {
		if _, err := compileQuery(query, resolve); err == nil {
			t.Errorf("compileQuery(%q) expected error, got nil", query)
		}
	}
}

func TestNormalizeQuery(t *testing.T) {
	tests := map[string]string{
		"if rpm > 3000":            "rpm > 3000",
		"a and b or c":             "a && b || c",
		"not (a > 1)":              "! (a > 1)",
		"boost ~ 1.2":              "__approx(boost, 1.2)",
		"In.p ~ 1.0 and rpm > 100": "__approx(In.p, 1.0) && rpm > 100",
		"load in [40, 60]":         "__in(load, 40, 60)",
		"rpm in [800] or load > 5": "__in(rpm, 800) || load > 5",
		// "in" inside a name must survive (no leading whitespace before "in").
		"MainInput > 1": "MainInput > 1",
		// "and"/"or" inside names must survive.
		"Sensor > 1 and Brand < 2": "Sensor > 1 && Brand < 2",
	}
	for in, want := range tests {
		if got := normalizeQuery(in); got != want {
			t.Errorf("normalizeQuery(%q) = %q, want %q", in, got, want)
		}
	}
}
