package core

import (
	"github.com/prometheus/prometheus/pkg/labels"
)

type (
	// ACL provides the Prometheus LabelMatchers
	ACL interface {
		// GetLabelMatchers returns the LabelMatchers for a metric name
		GetLabelMatchers(string) []*labels.Matcher
	}
)
