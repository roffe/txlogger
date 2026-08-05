package matrixbuilder

// This file implements the matrix builder's filter query language. Rather than
// hand-rolling a lexer and parser we lean on Go's own expression parser
// (go/parser) and walk the resulting syntax tree (go/ast). The query grammar is
// deliberately a subset of Go expressions, so once we normalise a few
// human-friendly tokens into their Go equivalents the standard library does the
// hard work of tokenising, applying operator precedence and handling ()
// grouping for us. We then "compile" the AST into a tree of small closures that
// can be evaluated cheaply once per log sample.
//
// Normalisation maps the bits that aren't valid Go onto bits that are:
//
//	if (rpm > 3000 and load > 50) or boost ~ 1.2
//	   (rpm > 3000 && load > 50) || __approx(boost, 1.2)
//
//   - a leading "if" is stripped (it reads nicely but carries no meaning)
//   - the keywords "and"/"or"/"not" become "&&"/"||"/"!"
//   - the approximately-equal operator "a ~ b" becomes a call "__approx(a, b)"
//   - the membership test "a in [x, y, z]" becomes a call "__in(a, x, y, z)"
//
// Series names are written verbatim. A bare name (m_Request) parses as an
// *ast.Ident; a dotted name (ActualIn.n_Engine) parses as an *ast.SelectorExpr,
// which we flatten back into the original dotted string.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strconv"
	"strings"
)

// boolNode evaluates a condition for the sample at index i.
type boolNode func(i int) bool

// numNode evaluates a value (a series reading, a literal or arithmetic of them)
// for the sample at index i. ok is false when a referenced series is missing or
// NaN at that sample, which makes every comparison using it fail — matching the
// "drop incomplete samples" behaviour of the older per-row rules.
type numNode func(i int) (float64, bool)

// queryFilter is a compiled query. Eval reports whether a sample passes; Series
// lists every series the query references, so the UI can warn about names that
// aren't present in the loaded logs.
type queryFilter struct {
	eval   boolNode
	series []string
}

func (q *queryFilter) Eval(i int) bool  { return q.eval(i) }
func (q *queryFilter) Series() []string { return q.series }

var (
	// A leading "if " (any case) is cosmetic and dropped. \b keeps "iffy" intact.
	leadingIfRe = regexp.MustCompile(`(?i)^if\b\s*`)
	// "a ~ b" -> "__approx(a, b)". Both operands are simple atoms: a series name
	// (optionally dotted) or a number (optionally negative/decimal). This runs
	// before the keyword swaps so the operands are still in their raw form.
	approxRe = regexp.MustCompile(`([A-Za-z_][\w.]*|-?\d[\d.]*)\s*~\s*(-?\d[\d.]*|[A-Za-z_][\w.]*)`)
	// "a in [x, y, z]" -> "__in(a, x, y, z)". The left operand is a simple atom
	// (a series name or number); the bracketed list is captured verbatim and
	// dropped in as the remaining call args, so go/parser tokenises the members.
	// Runs before the keyword swaps so the operand is still in its raw form.
	inRe = regexp.MustCompile(`(?i)([A-Za-z_][\w.]*|-?\d[\d.]*)\s+in\s*\[([^\]]*)\]`)
	// Keyword operators -> Go operators. \b stops us from rewriting these letters
	// when they appear inside a series name (e.g. "Sensor" keeps its "or").
	andRe = regexp.MustCompile(`(?i)\band\b`)
	orRe  = regexp.MustCompile(`(?i)\bor\b`)
	notRe = regexp.MustCompile(`(?i)\bnot\b`)
	// go/parser prefixes messages with a "line:col: " position into our
	// normalised string, which is meaningless to the user; strip it.
	parserPosRe = regexp.MustCompile(`^\d+:\d+:\s*`)
)

// normalizeQuery rewrites the human-friendly query into a valid Go expression
// string that go/parser can handle.
func normalizeQuery(src string) string {
	s := strings.TrimSpace(src)
	if s == "" {
		return ""
	}
	s = leadingIfRe.ReplaceAllString(s, "")
	s = approxRe.ReplaceAllString(s, "__approx($1, $2)")
	s = inRe.ReplaceAllString(s, "__in($1, $2)")
	s = andRe.ReplaceAllString(s, "&&")
	s = orRe.ReplaceAllString(s, "||")
	s = notRe.ReplaceAllString(s, "!")
	return strings.TrimSpace(s)
}

// compileQuery parses and compiles src into a queryFilter. resolve supplies a
// series' value at a sample (returning ok=false for a missing/NaN reading).
// Unknown series are not an error here — they simply never pass — so a query
// can be validated before any logs are loaded.
func compileQuery(src string, resolve func(series string, i int) (float64, bool)) (*queryFilter, error) {
	norm := normalizeQuery(src)
	if norm == "" {
		return nil, fmt.Errorf("empty query")
	}
	expr, err := parser.ParseExpr(norm)
	if err != nil {
		return nil, fmt.Errorf("syntax error: %s", cleanParseError(err))
	}
	c := &queryCompiler{resolve: resolve, seen: make(map[string]bool)}
	node, err := c.compileBool(expr)
	if err != nil {
		return nil, err
	}
	return &queryFilter{eval: node, series: c.series}, nil
}

// queryCompiler walks the AST, building closures and collecting referenced
// series names along the way.
type queryCompiler struct {
	resolve func(series string, i int) (float64, bool)
	seen    map[string]bool
	series  []string
}

// compileBool compiles an expression that must yield a boolean: a comparison, an
// approx() call, a && / || combination, a ! negation, or any of those grouped in
// parentheses.
func (c *queryCompiler) compileBool(e ast.Expr) (boolNode, error) {
	switch v := e.(type) {
	case *ast.ParenExpr:
		return c.compileBool(v.X)
	case *ast.UnaryExpr:
		if v.Op != token.NOT {
			return nil, fmt.Errorf("%q can't start a condition", v.Op)
		}
		inner, err := c.compileBool(v.X)
		if err != nil {
			return nil, err
		}
		return func(i int) bool { return !inner(i) }, nil
	case *ast.CallExpr:
		if id, ok := v.Fun.(*ast.Ident); ok && id.Name == "__in" {
			return c.compileIn(v)
		}
		return c.compileApprox(v)
	case *ast.BinaryExpr:
		switch v.Op {
		case token.LAND:
			return c.combine(v, func(a, b bool) bool { return a && b })
		case token.LOR:
			return c.combine(v, func(a, b bool) bool { return a || b })
		case token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ:
			return c.compileCompare(v)
		default:
			return nil, fmt.Errorf("operator %q can't join conditions; use and/or", v.Op)
		}
	}
	return nil, fmt.Errorf("expected a condition (e.g. rpm > 3000), got %s", exprString(e))
}

// combine compiles both sides of a && / || node and merges them with op.
func (c *queryCompiler) combine(v *ast.BinaryExpr, op func(a, b bool) bool) (boolNode, error) {
	l, err := c.compileBool(v.X)
	if err != nil {
		return nil, err
	}
	r, err := c.compileBool(v.Y)
	if err != nil {
		return nil, err
	}
	return func(i int) bool { return op(l(i), r(i)) }, nil
}

// compileCompare compiles a comparison "value op value". If either side is
// missing/NaN at a sample the comparison is false.
func (c *queryCompiler) compileCompare(v *ast.BinaryExpr) (boolNode, error) {
	l, err := c.compileNum(v.X)
	if err != nil {
		return nil, err
	}
	r, err := c.compileNum(v.Y)
	if err != nil {
		return nil, err
	}
	op := v.Op
	return func(i int) bool {
		a, ok := l(i)
		if !ok {
			return false
		}
		b, ok := r(i)
		if !ok {
			return false
		}
		switch op {
		case token.EQL:
			return a == b
		case token.NEQ:
			return a != b
		case token.LSS:
			return a < b
		case token.LEQ:
			return a <= b
		case token.GTR:
			return a > b
		case token.GEQ:
			return a >= b
		}
		return false
	}, nil
}

// compileApprox compiles the synthesised __approx(a, b) call that backs the "~"
// operator, reusing the same epsilon window as the per-row "~" rule.
func (c *queryCompiler) compileApprox(call *ast.CallExpr) (boolNode, error) {
	id, ok := call.Fun.(*ast.Ident)
	if !ok || id.Name != "__approx" {
		return nil, fmt.Errorf("unknown function %s(...)", exprString(call.Fun))
	}
	if len(call.Args) != 2 {
		return nil, fmt.Errorf("~ needs a value on each side")
	}
	l, err := c.compileNum(call.Args[0])
	if err != nil {
		return nil, err
	}
	r, err := c.compileNum(call.Args[1])
	if err != nil {
		return nil, err
	}
	return func(i int) bool {
		a, ok := l(i)
		if !ok {
			return false
		}
		b, ok := r(i)
		if !ok {
			return false
		}
		return approxEqual(a, b)
	}, nil
}

// compileIn compiles the synthesised __in(value, a, b, ...) call that backs the
// "in [...]" membership test. It passes when value equals any list member. A
// missing/NaN value (or list member) is skipped, matching the "drop incomplete
// samples" behaviour of every other comparison.
func (c *queryCompiler) compileIn(call *ast.CallExpr) (boolNode, error) {
	if len(call.Args) < 2 {
		return nil, fmt.Errorf("in needs a value and a non-empty list, e.g. rpm in [800, 900]")
	}
	val, err := c.compileNum(call.Args[0])
	if err != nil {
		return nil, err
	}
	set := make([]numNode, 0, len(call.Args)-1)
	for _, arg := range call.Args[1:] {
		member, err := c.compileNum(arg)
		if err != nil {
			return nil, err
		}
		set = append(set, member)
	}
	return func(i int) bool {
		a, ok := val(i)
		if !ok {
			return false
		}
		for _, member := range set {
			if b, ok := member(i); ok && a == b {
				return true
			}
		}
		return false
	}, nil
}

// compileNum compiles an expression that must yield a number: a literal, a
// series reference, arithmetic of those, or any of them grouped/negated.
func (c *queryCompiler) compileNum(e ast.Expr) (numNode, error) {
	switch v := e.(type) {
	case *ast.ParenExpr:
		return c.compileNum(v.X)
	case *ast.BasicLit:
		if v.Kind != token.INT && v.Kind != token.FLOAT {
			return nil, fmt.Errorf("expected a number, got %s", v.Value)
		}
		f, err := strconv.ParseFloat(v.Value, 64)
		if err != nil {
			return nil, fmt.Errorf("bad number %q", v.Value)
		}
		return func(int) (float64, bool) { return f, true }, nil
	case *ast.Ident:
		return c.seriesNode(v.Name), nil
	case *ast.SelectorExpr:
		name, ok := dottedName(v)
		if !ok {
			return nil, fmt.Errorf("unsupported series reference %s", exprString(v))
		}
		return c.seriesNode(name), nil
	case *ast.UnaryExpr:
		switch v.Op {
		case token.SUB:
			inner, err := c.compileNum(v.X)
			if err != nil {
				return nil, err
			}
			return func(i int) (float64, bool) { x, ok := inner(i); return -x, ok }, nil
		case token.ADD:
			return c.compileNum(v.X)
		case token.NOT:
			// "!" binds tighter than comparison, so "not rpm > 3000" parses as
			// "(not rpm) > 3000". Point the user at the parenthesised form.
			return nil, fmt.Errorf("not negates a condition — wrap it, e.g. not (rpm > 3000)")
		}
		return nil, fmt.Errorf("%q can't be applied to a value", v.Op)
	case *ast.BinaryExpr:
		return c.compileArith(v)
	}
	return nil, fmt.Errorf("expected a series name or number, got %s", exprString(e))
}

// compileArith compiles +, -, * and / between two values. Division by zero
// yields ok=false rather than an infinity.
func (c *queryCompiler) compileArith(v *ast.BinaryExpr) (numNode, error) {
	op := v.Op
	switch op {
	case token.ADD, token.SUB, token.MUL, token.QUO:
	default:
		return nil, fmt.Errorf("operator %q can't be used in a value", op)
	}
	l, err := c.compileNum(v.X)
	if err != nil {
		return nil, err
	}
	r, err := c.compileNum(v.Y)
	if err != nil {
		return nil, err
	}
	return func(i int) (float64, bool) {
		a, ok := l(i)
		if !ok {
			return 0, false
		}
		b, ok := r(i)
		if !ok {
			return 0, false
		}
		switch op {
		case token.ADD:
			return a + b, true
		case token.SUB:
			return a - b, true
		case token.MUL:
			return a * b, true
		case token.QUO:
			if b == 0 {
				return 0, false
			}
			return a / b, true
		}
		return 0, false
	}, nil
}

// seriesNode builds a value node that reads the named series at a sample, and
// records the name as referenced (deduplicated, in first-seen order).
func (c *queryCompiler) seriesNode(name string) numNode {
	if !c.seen[name] {
		c.seen[name] = true
		c.series = append(c.series, name)
	}
	resolve := c.resolve
	return func(i int) (float64, bool) { return resolve(name, i) }
}

// dottedName flattens a selector chain (a.b.c) back into its original dotted
// string. Only plain identifiers may appear in the chain.
func dottedName(e ast.Expr) (string, bool) {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name, true
	case *ast.SelectorExpr:
		base, ok := dottedName(v.X)
		if !ok {
			return "", false
		}
		return base + "." + v.Sel.Name, true
	}
	return "", false
}

// exprString renders a small, friendly fragment of an expression for error
// messages, covering just the node kinds this grammar can produce.
func exprString(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.BasicLit:
		return v.Value
	case *ast.SelectorExpr:
		if n, ok := dottedName(v); ok {
			return n
		}
		return "selector"
	case *ast.ParenExpr:
		return "(" + exprString(v.X) + ")"
	case *ast.UnaryExpr:
		return v.Op.String() + exprString(v.X)
	case *ast.BinaryExpr:
		return exprString(v.X) + " " + v.Op.String() + " " + exprString(v.Y)
	case *ast.CallExpr:
		return exprString(v.Fun) + "(...)"
	}
	return "expression"
}

// cleanParseError trims go/parser's "line:col: " position prefix, which refers
// to our normalised string and would only confuse the user.
func cleanParseError(err error) string {
	return parserPosRe.ReplaceAllString(err.Error(), "")
}
