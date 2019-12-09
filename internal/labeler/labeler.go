package labeler

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/promql"

	"github.com/bitsbeats/prometheus-acls/internal/core"
)

type (
	// Labeler provides the relabeling functions and thracks the metrics
	Labeler struct {
		labelerDurationHist prometheus.Histogram
		dedupeDurationHist  prometheus.Histogram
		queryParseHist      prometheus.Histogram
		promProxyHist       prometheus.Histogram
	}
)

// NoneLabelMatcher is a prometheus label matcher that fails for all metrics
var NoneLabelMatcher = []*labels.Matcher{{
	Name:  "__",
	Value: "none",
	Type:  labels.MatchEqual,
}}

// ParseLabels uses Prometheus promql library to parse a string of Prometheus labels
// into LaberMatchers
func ParseLabels(query string) (labelMatchers []*labels.Matcher, err error) {
	expr, err := promql.ParseExpr(fmt.Sprintf("{%s}", query))
	if err != nil {
		return nil, err
	}
	casted, ok := expr.(*promql.VectorSelector)
	if !ok {
		return nil, fmt.Errorf("unable to load labels from '%s'", query)
	}
	return casted.LabelMatchers, nil
}

// MustParseLabels is a ParseLabels version that panics on error
func MustParseLabels(query string) (labelMatchers []*labels.Matcher) {
	lm, err := ParseLabels(query)
	if err != nil {
		panic(err.Error())
	}
	return lm
}

// NewLabeler creates a new instance of *Labeler
func NewLabeler() (l *Labeler) {
	l = &Labeler{
		labelerDurationHist: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "prometheus_acls_labeler_duration_seconds",
			Help:    "A Histogram which tracks the time taken to add acl labels and dedupe.",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 11),
		}),
		dedupeDurationHist: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "prometheus_acls_dedupe_duration_seconds",
			Help:    "A Histogram which tracks the time taken to dedupe queries.",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 11),
		}),
		queryParseHist: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "prometheus_acls_parser_duration_seconds",
			Help:    "A Histogram which tracks the time taken to parse Prometheus queries.",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 11),
		}),
		promProxyHist: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "prometheus_acls_reverseproxy_response_seconds",
			Help:    "A Histogram that tracks the response latency of the upstream Prometheus",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 16),
		}),
	}
	prometheus.MustRegister(l.labelerDurationHist, l.queryParseHist, l.promProxyHist)
	return
}

// AddLabels reversively walks through a promql.Expr and adds the LabelMatches provided by
// core.ACL to every metric
//
// This function tries to follow the same flow as Promtheus eval
// https://github.com/prometheus/prometheus/blob/master/promql/engine.go#L923
func (l *Labeler) AddLabels(expr promql.Expr, acl core.ACL) (labeled promql.Expr) {
	switch casted := expr.(type) {
	case *promql.AggregateExpr:
		casted.Expr = l.AddLabels(casted.Expr, acl)
		return casted
	case *promql.Call:
		for i, expr := range casted.Args {
			casted.Args[i] = l.AddLabels(expr, acl)
		}
		return casted
	case *promql.ParenExpr:
		casted.Expr = l.AddLabels(casted.Expr, acl)
		return casted
	case *promql.UnaryExpr:
		casted.Expr = l.AddLabels(casted.Expr, acl)
		return casted
	case *promql.BinaryExpr:
		casted.RHS = l.AddLabels(casted.RHS, acl)
		casted.LHS = l.AddLabels(casted.LHS, acl)
		return casted
	case *promql.NumberLiteral:
		return expr
	case *promql.VectorSelector:
		matchers := append(casted.LabelMatchers, acl.GetLabelMatchers(casted.Name)...)
		casted.LabelMatchers = l.DedupeMatchers(matchers)
		return casted
	case *promql.MatrixSelector:
		matchers := append(casted.LabelMatchers, acl.GetLabelMatchers(casted.Name)...)
		casted.LabelMatchers = l.DedupeMatchers(matchers)
		return casted
	case *promql.SubqueryExpr:
		casted.Expr = l.AddLabels(casted.Expr, acl)
		return casted
	}
	return expr
}

// DedupeMatchers tries to find Matchers that do the same stuff and remove the more cpu intersive
// version.
func (l *Labeler) DedupeMatchers(matchers []*labels.Matcher) []*labels.Matcher {
	deduped := []*labels.Matcher{}
	exactMatchers := []*labels.Matcher{}
	exactNotMatchers := []*labels.Matcher{}
	reMatchers := []*labels.Matcher{}
	reNotMatchers := []*labels.Matcher{}
	for _, matcher := range matchers {
		switch matcher.Type {
		case labels.MatchEqual:
			exactMatchers = append(exactMatchers, matcher)
		case labels.MatchNotEqual:
			exactNotMatchers = append(exactNotMatchers, matcher)

		case labels.MatchRegexp:
			reMatchers = append(reMatchers, matcher)
		case labels.MatchNotRegexp:
			reNotMatchers = append(reNotMatchers, matcher)
		}
	}

outerNotExact:
	for i, notMatcher := range exactNotMatchers {
		for _, matcher := range exactMatchers {
			if notMatcher.Name == matcher.Name && notMatcher.Value != matcher.Value {
				// notMatcher already satisfied by exactMatcher
				continue outerNotExact
			}
		}
		for _, notMatcher2 := range exactNotMatchers[i+1:] {
			if notMatcher.Name == notMatcher2.Name && notMatcher.Value == notMatcher2.Value {
				// doubled
				continue outerNotExact
			}
		}
		deduped = append(deduped, notMatcher)
	}

outerExact:
	for i, matcher := range exactMatchers {
		for _, matcher2 := range exactMatchers[i+1:] {
			if matcher.Name == matcher2.Name {
				if matcher.Value == matcher2.Value {
					// doubled
					continue outerExact
				} else {
					// one label cant have two results at the same time
					return NoneLabelMatcher
				}
			}
		}
		deduped = append(deduped, matcher)
	}

outerNotRe:
	for i, reNotMatcher := range reNotMatchers {
		for _, matcher := range exactMatchers {
			if reNotMatcher.Name == matcher.Name {
				if reNotMatcher.Matches(matcher.Value) {
					// regex not match already by static matcher done
					continue outerNotRe
				} else {
					// regex does not match the leftovers of static match
					return NoneLabelMatcher
				}
			}
		}
		for _, reNotMatcher2 := range reNotMatchers[i+1:] {
			if reNotMatcher.Name == reNotMatcher2.Name && reNotMatcher.Value == reNotMatcher2.Value {
				// doubled
				continue outerNotRe
			}
		}
		deduped = append(deduped, reNotMatcher)
	}

outerRe:
	for i, reMatcher := range reMatchers {
		for _, matcher := range exactMatchers {
			if matcher.Name == reMatcher.Name {
				if reMatcher.Matches(matcher.Value) {
					// drop reMatcher that is already satisfied by matcher
					continue outerRe
				} else {
					// // regex does not match the leftovers of static match
					return NoneLabelMatcher
				}

			}
		}
		for _, reMatcher2 := range reMatchers[i+1:] {
			if reMatcher.Name == reMatcher2.Name && reMatcher.Value == reMatcher2.Value {
				// doubled
				continue outerRe
			}
		}
		deduped = append(deduped, reMatcher)
	}
	return deduped
}
