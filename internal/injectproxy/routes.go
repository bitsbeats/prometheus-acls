package injectproxy

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/promql/parser"

	"github.com/bitsbeats/prometheus-acls/internal/api"
)

var LabelValuesRegexp = regexp.MustCompile("/api/v1/label/.+/values$")

type ctxKey int

const (
	KeyLabel ctxKey = iota
)

type CtxValue struct {
	Matcher *labels.Matcher
	Admin   bool
}

const (
	queryParam    = "query"
	matchersParam = "match[]"
)

type routes struct {
	upstream *url.URL
	handler  http.Handler
	label    string

	modifiers      map[string]func(*http.Response) error
	errorOnReplace bool
}

func NewRoutes(upstream *url.URL, label string, errorOnReplace bool) (*routes, error) {
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	proxy.Transport = &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   600 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	r := &routes{upstream: upstream, handler: proxy, label: label, errorOnReplace: errorOnReplace}

	return r, nil
}

func (r *routes) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	val, ok := ctx.Value(KeyLabel).(*CtxValue)
	if !ok {
		api.RespondError(w, &api.ApiError{Typ: api.ErrorInternal, Err: errors.New("can't find label matcher in the context")}, nil)
		return
	}

	if val.Matcher == nil && !val.Admin {
		api.RespondError(w, &api.ApiError{Typ: api.ErrorNoPermission, Err: errors.New("user doesn't have access to Thanos")}, nil)
		return
	}

	req.Host = r.upstream.Hostname()
	path := req.URL.EscapedPath()
	switch {
	case strings.HasSuffix(path, "/api/v1/query") ||
		strings.HasSuffix(path, "/api/v1/query_range") ||
		strings.HasSuffix(path, "/api/v1/query_exemplars"):
		r.query(w, req)

	case strings.HasSuffix(path, "/api/v1/series") ||
		strings.HasSuffix(path, "/api/v1/labels") ||
		LabelValuesRegexp.MatchString(path):
		r.matcher(w, req)

	default:
		r.handler.ServeHTTP(w, req)
	}
}

func mustLabelMatcher(ctx context.Context) *labels.Matcher {
	val, ok := ctx.Value(KeyLabel).(*CtxValue)
	if !ok {
		panic(fmt.Sprintf("can't find the %q value in the context", KeyLabel))
	}

	return val.Matcher
}

func (r *routes) query(w http.ResponseWriter, req *http.Request) {
	var e Enforcer
	matcher := mustLabelMatcher(req.Context())
	if matcher == nil {
		e = NoopEnforcer{}
	} else {
		e = NewEnforcer(r.errorOnReplace, matcher)
	}

	// The `query` can come in the URL query string and/or the POST body.
	// For this reason, we need to try to enforce in both places.
	// Note: a POST request may include some values in the URL query string
	// and others in the body. If both locations include a `query`, then
	// enforce in both places.
	q, found1, err := enforceQueryValues(e, req.URL.Query())
	if err != nil {
		if _, ok := err.(IllegalLabelMatcherError); ok {
			api.RespondError(w, &api.ApiError{Typ: api.ErrorBadData, Err: err}, nil)
		}
		return
	}
	req.URL.RawQuery = q

	var found2 bool
	// Enforce the query in the POST body if needed.
	if req.Method == http.MethodPost {
		if err := req.ParseForm(); err != nil {
			return
		}
		q, found2, err = enforceQueryValues(e, req.PostForm)
		if err != nil {
			if _, ok := err.(IllegalLabelMatcherError); ok {
				api.RespondError(w, &api.ApiError{Typ: api.ErrorBadData, Err: err}, nil)
			}
			return
		}
		// We are replacing request body, close previous one (ParseForm ensures it is read fully and not nil).
		_ = req.Body.Close()
		req.Body = ioutil.NopCloser(strings.NewReader(q))
		req.ContentLength = int64(len(q))
	}

	// If no query was found, return early.
	if !found1 && !found2 {
		return
	}

	r.handler.ServeHTTP(w, req)
}

func enforceQueryValues(e Enforcer, v url.Values) (values string, noQuery bool, err error) {
	// If no values were given or no query is present,
	// e.g. because the query came in the POST body
	// but the URL query string was passed, then finish early.
	if v.Get(queryParam) == "" {
		return v.Encode(), false, nil
	}
	expr, err := parser.ParseExpr(v.Get(queryParam))
	if err != nil {
		return "", true, err
	}

	if err := e.EnforceNode(expr); err != nil {
		return "", true, err
	}

	v.Set(queryParam, expr.String())
	return v.Encode(), true, nil
}

// matcher ensures all the provided match[] if any has label injected. If none was provided, single matcher is injected.
// This works for non-query Prometheus APIs like: /api/v1/series, /api/v1/label/<name>/values, /api/v1/labels and /federate support multiple matchers.
// See e.g https://prometheus.io/docs/prometheus/latest/querying/api/#querying-metadata
func (r *routes) matcher(w http.ResponseWriter, req *http.Request) {
	matcher := mustLabelMatcher(req.Context())
	q := req.URL.Query()
	if err := injectMatcher(q, matcher); err != nil {
		return
	}
	req.URL.RawQuery = q.Encode()
	if req.Method == http.MethodPost {
		q = req.PostForm
		if q == nil {
			q = url.Values{}
		}
		if err := injectMatcher(q, matcher); err != nil {
			return
		}
		// We are replacing request body, close previous one (ParseForm ensures it is read fully and not nil).
		_ = req.Body.Close()
		newBody := q.Encode()
		req.Body = ioutil.NopCloser(strings.NewReader(newBody))
		req.ContentLength = int64(len(newBody))
	}
	r.handler.ServeHTTP(w, req)
}

func injectMatcher(q url.Values, matcher *labels.Matcher) error {
	if matcher == nil {
		return nil
	}
	matchers := q[matchersParam]
	if len(matchers) == 0 {
		q.Set(matchersParam, matchersToString(matcher))
	} else {
		// Inject label to existing matchers.
		for i, m := range matchers {
			ms, err := parser.ParseMetricSelector(m)
			if err != nil {
				return err
			}
			matchers[i] = matchersToString(append(ms, matcher)...)
		}
		q[matchersParam] = matchers
	}
	return nil
}

func matchersToString(ms ...*labels.Matcher) string {
	var el []string
	for _, m := range ms {
		el = append(el, m.String())
	}
	return fmt.Sprintf("{%v}", strings.Join(el, ","))
}
