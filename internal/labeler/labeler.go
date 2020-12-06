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
		casted.LabelMatchers = DedupeMatchers(matchers)
		return casted
	case *promql.MatrixSelector:
		matchers := append(casted.LabelMatchers, acl.GetLabelMatchers(casted.Name)...)
		casted.LabelMatchers = DedupeMatchers(matchers)
		return casted
	case *promql.SubqueryExpr:
		casted.Expr = l.AddLabels(casted.Expr, acl)
		return casted
	}
	return expr
}

