package labeler

import (
	"github.com/prometheus/prometheus/pkg/labels"
)

type labelMatcherDeduper struct {
	deduped          []*labels.Matcher
	exactMatchers    []*labels.Matcher
	exactNotMatchers []*labels.Matcher
	reMatchers       []*labels.Matcher
	reNotMatchers    []*labels.Matcher
}

// DedupeMatchers tries to find Matchers that do the same stuff and remove the more cpu intersive
// version.
func DedupeMatchers(matchers []*labels.Matcher) []*labels.Matcher {
	lmb := &labelMatcherDeduper{}
	for _, matcher := range matchers {
		switch matcher.Type {
		case labels.MatchEqual:
			lmb.exactMatchers = append(lmb.exactMatchers, matcher)
		case labels.MatchNotEqual:
			lmb.exactNotMatchers = append(lmb.exactNotMatchers, matcher)

		case labels.MatchRegexp:
			lmb.reMatchers = append(lmb.reMatchers, matcher)
		case labels.MatchNotRegexp:
			lmb.reNotMatchers = append(lmb.reNotMatchers, matcher)
		}
	}

	all, ok := lmb.dedupeExactNotMatchers()
	if !ok {
		return NoneLabelMatcher
	}

	deduped, ok := lmb.dedupeExactMatchers()
	if !ok {
		return NoneLabelMatcher
	}
	all = append(all, deduped...)

	deduped, ok = lmb.dedupeReNotMatchers()
	if !ok {
		return NoneLabelMatcher
	}
	all = append(all, deduped...)

	deduped, ok = lmb.dedupeReMatchers()
	if !ok {
		return NoneLabelMatcher
	}
	all = append(all, deduped...)

	return all
}

// dedupeExactNotMatchers removes all notMatchers that are already implicibly
// stated by exactMatchers or exist more than once
func (lmb *labelMatcherDeduper) dedupeExactNotMatchers() ([]*labels.Matcher, bool) {
	deduped := []*labels.Matcher{}
outerNotExact:
	for i, notMatcher := range lmb.exactNotMatchers {
		for _, matcher := range lmb.exactMatchers {
			if notMatcher.Name == matcher.Name && notMatcher.Value != matcher.Value {
				// notMatcher already satisfied by exactMatcher
				continue outerNotExact
			}
		}
		for _, notMatcher2 := range lmb.exactNotMatchers[i+1:] {
			if notMatcher.Name == notMatcher2.Name && notMatcher.Value == notMatcher2.Value {
				// doubled
				continue outerNotExact
			}
		}
		deduped = append(deduped, notMatcher)
	}
	return deduped, true
}

// dedupeExactMatchers removes all Matches that are doubed or invalidates the
// query if there are two conflicing ones
func (lmb *labelMatcherDeduper) dedupeExactMatchers() ([]*labels.Matcher, bool) {
	deduped := []*labels.Matcher{}
outerExact:
	for i, matcher := range lmb.exactMatchers {
		for _, matcher2 := range lmb.exactMatchers[i+1:] {
			if matcher.Name == matcher2.Name {
				if matcher.Value == matcher2.Value {
					// doubled
					continue outerExact
				} else {
					// one label cant have two results at the same time
					return nil, false
				}
			}
		}
		deduped = append(deduped, matcher)
	}
	return deduped, true
}

// dedupeReNotMatchers removes all regex matchers that are ensures by static matchers
// or invalidates the query if there are conflicting static ones
func (lmb *labelMatcherDeduper) dedupeReNotMatchers() ([]*labels.Matcher, bool) {
	deduped := []*labels.Matcher{}
outerNotRe:
	for i, reNotMatcher := range lmb.reNotMatchers {
		for _, matcher := range lmb.exactMatchers {
			if reNotMatcher.Name == matcher.Name {
				if reNotMatcher.Matches(matcher.Value) {
					// regex not match already by static matcher done
					continue outerNotRe
				} else {
					// regex does not match the leftovers of static match
					return nil, false
				}
			}
		}
		for _, reNotMatcher2 := range lmb.reNotMatchers[i+1:] {
			if reNotMatcher.Name == reNotMatcher2.Name && reNotMatcher.Value == reNotMatcher2.Value {
				// doubled
				continue outerNotRe
			}
		}
		deduped = append(deduped, reNotMatcher)
	}
	return deduped, true
}

// dedupeReMatchers removes all regex matchers that are ensured by a static
// matcher or invalidates the query if there are conflicing static matchers
func (lmb *labelMatcherDeduper) dedupeReMatchers() ([]*labels.Matcher, bool) {
	deduped := []*labels.Matcher{}
outerRe:
	for i, reMatcher := range lmb.reMatchers {
		for _, matcher := range lmb.exactMatchers {
			if matcher.Name == reMatcher.Name {
				if reMatcher.Matches(matcher.Value) {
					// drop reMatcher that is already satisfied by matcher
					continue outerRe
				} else {
					// // regex does not match the leftovers of static match
					return nil, false
				}

			}
		}
		for _, reMatcher2 := range lmb.reMatchers[i+1:] {
			if reMatcher.Name == reMatcher2.Name && reMatcher.Value == reMatcher2.Value {
				// doubled
				continue outerRe
			}
		}
		deduped = append(deduped, reMatcher)
	}
	return deduped, true
}
