package config

import (
	"fmt"
	"github.com/prometheus/prometheus/pkg/labels"
	"regexp"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/bitsbeats/prometheus-acls/internal/labeler"
)

type (
	// OidcRole is a string to identify a OIDC role from the OAUTH provider
	OidcRole string

	// MetricName is a string to identify a Prometheus metric
	MetricName string

	// NamedACL hold the LabelMatchers for a specific MetricName
	NamedACL map[MetricName][]*labels.Matcher

	// RegexACL holds the LabelMatchers for all MetricNames that match Regexp
	RegexACL struct {
		Regexp        *regexp.Regexp
		LabelMatchers []*labels.Matcher
	}

	// ACL holds the parsed Named and Regex metricName to LabelMatchers
	ACL struct {
		Named NamedACL
		Regex []RegexACL
	}

	// ACLMap is used to look up OidcRole for its configures ACL
	ACLMap map[OidcRole]*ACL
)

// None is a special LabelMatcher that matches for no metric, used to deny access to a metric
var None = labeler.MustParseLabels("__=\"none\"")

// GetACL fetches the ACLs for a specific OidcRole
func (a ACLMap) GetACL(role string) (*ACL, bool) {
	acl, ok := a[OidcRole(role)]
	return acl, ok
}

// GetDenyACL provides a empty ACL that deny access to any metric
func (a ACLMap) GetDenyACL() *ACL {
	return &ACL{}
}

// ParseAndStoreACL parses the metricName if its a NamedACL or a RegexACL and the query
// for all supported query types (see parseLabels)
func (a *ACL) ParseAndStoreACL(metricName string, query interface{}) (err error) {
	lm, err := a.parseLabels(query)
	if err != nil {
		return err
	}
	if strings.HasPrefix(metricName, "re!") {
		expr := strings.TrimPrefix(metricName, "re!")
		if !strings.HasPrefix(expr, "^") {
			log.WithField(
				"expr", expr,
			).Warn("consider matching for label start with '^'")
		}
		r, err := regexp.Compile(expr)
		if err != nil {
			return err
		}
		a.Regex = append(a.Regex, RegexACL{
			Regexp:        r,
			LabelMatchers: lm,
		})
	}
	a.Named[MetricName(metricName)] = lm
	return
}

// GetLabelMatchers checks in order against exact the NamedACL, a RegexACL and the
// fallback '*' ACL and returns the corresponding LabelMatchers
func (a *ACL) GetLabelMatchers(metricName string) []*labels.Matcher {
	labelMatchers, ok := a.Named[MetricName(metricName)]
	if ok {
		log.WithField("labelMatchers", labelMatchers).Debug("added labels")
		return labelMatchers
	}
	for _, racl := range a.Regex {
		if racl.Regexp.MatchString(metricName) {
			return racl.LabelMatchers
		}
	}
	labelMatchers, ok = a.Named["*"]
	if ok {
		log.WithField("labelMatchers", labelMatchers).Debug("added labels")
		return labelMatchers
	}
	labelMatchers = None
	log.WithField("labelMatchers", labelMatchers).Debug("added labels")
	return labelMatchers
}

// parseLabels parses the query and returns the resulting LabelMatchers. Currently supports
// nil, empty string and prometheus label query as query
func (a *ACL) parseLabels(query interface{}) (lm []*labels.Matcher, err error) {
	switch casted := query.(type) {
	case nil:
		return None, nil
	case string:
		if query == "" {
			return []*labels.Matcher{}, nil
		}
		lm, err = labeler.ParseLabels(casted)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unable to parse config: %T is not a valid query", casted)
	}
	return
}
