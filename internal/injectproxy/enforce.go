package injectproxy

import (
	"fmt"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/promql/parser"
)

type Enforcer interface {
	EnforceNode(node parser.Node) error
}

type NoopEnforcer struct{}

func (e NoopEnforcer) EnforceNode(_ parser.Node) error { return nil }

type MatchersEnforcer struct {
	labelMatchers  map[string]*labels.Matcher
	errorOnReplace bool
}

func NewEnforcer(errorOnReplace bool, ms ...*labels.Matcher) *MatchersEnforcer {
	entries := make(map[string]*labels.Matcher)

	for _, matcher := range ms {
		entries[matcher.Name] = matcher
	}

	return &MatchersEnforcer{
		labelMatchers:  entries,
		errorOnReplace: errorOnReplace,
	}
}

type IllegalLabelMatcherError struct {
	msg string
}

func (e IllegalLabelMatcherError) Error() string { return e.msg }

func newIllegalLabelMatcherError(existing string, replacement string) IllegalLabelMatcherError {
	return IllegalLabelMatcherError{
		msg: fmt.Sprintf("allowed label matcher (%s) conflicts with given label matcher (%s)", existing, replacement),
	}
}

// EnforceNode walks the given node recursively
// and enforces the given label enforcer on it.
//
// Whenever a parser.MatrixSelector or parser.VectorSelector AST node is found,
// their label enforcer is being potentially modified.
// If a node's label matcher has the same name as a label matcher
// of the given enforcer, then it will be replaced.
func (ms MatchersEnforcer) EnforceNode(node parser.Node) error {
	switch n := node.(type) {
	case *parser.EvalStmt:
		if err := ms.EnforceNode(n.Expr); err != nil {
			return err
		}

	case parser.Expressions:
		for _, e := range n {
			if err := ms.EnforceNode(e); err != nil {
				return err
			}
		}

	case *parser.AggregateExpr:
		if err := ms.EnforceNode(n.Expr); err != nil {
			return err
		}

	case *parser.BinaryExpr:
		if err := ms.EnforceNode(n.LHS); err != nil {
			return err
		}

		if err := ms.EnforceNode(n.RHS); err != nil {
			return err
		}

	case *parser.Call:
		if err := ms.EnforceNode(n.Args); err != nil {
			return err
		}

	case *parser.SubqueryExpr:
		if err := ms.EnforceNode(n.Expr); err != nil {
			return err
		}

	case *parser.ParenExpr:
		if err := ms.EnforceNode(n.Expr); err != nil {
			return err
		}

	case *parser.UnaryExpr:
		if err := ms.EnforceNode(n.Expr); err != nil {
			return err
		}

	case *parser.NumberLiteral, *parser.StringLiteral:
	// nothing to do

	case *parser.MatrixSelector:
		// inject labelselector
		if vs, ok := n.VectorSelector.(*parser.VectorSelector); ok {
			var err error
			vs.LabelMatchers, err = ms.EnforceMatchers(vs.LabelMatchers)
			if err != nil {
				return err
			}
		}

	case *parser.VectorSelector:
		// inject labelselector
		var err error
		n.LabelMatchers, err = ms.EnforceMatchers(n.LabelMatchers)
		if err != nil {
			return err
		}

	default:
		panic(fmt.Errorf("parser.Walk: unhandled node type %T", n))
	}

	return nil
}

// EnforceMatchers appends the configured label matcher if not present.
// If the label matcher that is to be injected is present (by labelname) but
// different (either by match type or value) the behavior depends on the
// errorOnReplace variable. If errorOnReplace is true an error is returned,
// otherwise the label matcher is silently replaced.
func (ms MatchersEnforcer) EnforceMatchers(targets []*labels.Matcher) ([]*labels.Matcher, error) {
	var res []*labels.Matcher

	matchers := make(map[string]*labels.Matcher, len(ms.labelMatchers))
	for k, v := range ms.labelMatchers {
		matchers[k] = v
	}
	for _, target := range targets {
		if matcher, ok := ms.labelMatchers[target.Name]; ok {
			if !matcher.Matches(target.Value) {
				return res, newIllegalLabelMatcherError(matcher.String(), target.String())
			}
			delete(matchers, target.Name)
		}

		res = append(res, target)
	}

	for _, enforcedMatcher := range matchers {
		res = append(res, enforcedMatcher)
	}

	return res, nil
}
