package labeler

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/prometheus/prometheus/promql"
	log "github.com/sirupsen/logrus"

	"github.com/bitsbeats/prometheus-acls/internal/core"
	"github.com/bitsbeats/prometheus-acls/internal/prom"
)

// PromACLMiddlewareFor generates a Middleware for a URL that modifies Prometheus Queries
// by injecting additional Labels. These labels are provided by a core.ACL interface via
// the requests Context
func (l *Labeler) PromACLMiddlewareFor(u *url.URL) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			modified := false
			r.Host = u.Hostname()

			if r.URL.EscapedPath() == "/api/v1/query" || r.URL.EscapedPath() == "/api/v1/query_range" {
				// lookup acl in context
				acl, ok := r.Context().Value("acl").(core.ACL)
				if !ok {
					msg := fmt.Sprintf("unable to load acl from context: not found")
					prom.SendError(w, r, msg, http.StatusInternalServerError, nil)
					return
				}

				// manipulate post parameters
				err := r.ParseForm()
				if err != nil {
					msg := fmt.Sprintf("unable to parse form: %s", err)
					prom.SendError(w, r, msg, http.StatusInternalServerError, nil)
					return
				}
				subModified, err := l.labelize(&r.PostForm, acl)
				if err != nil {
					msg := fmt.Sprintf("unable to parse prometheus query: %s", err)
					prom.SendError(w, r, msg, http.StatusInternalServerError, nil)
					return
				}
				modified = modified || subModified
				newBody := strings.NewReader(r.PostForm.Encode())
				r.ContentLength = newBody.Size()
				r.Body = ioutil.NopCloser(newBody)

				// manipulate get parameters
				getParams := r.URL.Query()
				subModified, err = l.labelize(&getParams, acl)
				if err != nil {
					msg := fmt.Sprintf("unable to parse prometheus query: %s", err)
					prom.SendError(w, r, msg, http.StatusInternalServerError, nil)
					return
				}
				modified = modified || subModified

				// manipulate url
				r.URL, err = url.Parse(fmt.Sprintf(
					"%s://%s%s?%s",
					u.Scheme, u.Host, r.URL.EscapedPath(), getParams.Encode(),
				))
				if err != nil {
					msg := fmt.Sprintf("unable to parse prometheus query: %s", err)
					prom.SendError(w, r, msg, http.StatusInternalServerError, nil)
					return
				}

			}

			// serve the request
			start := time.Now()
			next.ServeHTTP(w, r)
			l.promProxyHist.Observe(time.Since(start).Seconds())

			// log
			log.WithFields(log.Fields{
				"modified": modified,
				"method":   r.Method,
			}).Info(r.URL.String())
		})
	}
}

// labelize modifies a prometheus query to inject labels based on acl
func (l *Labeler) labelize(params *url.Values, acl core.ACL) (modified bool, err error) {
	for key, value := range *params {
		if key == "query" {
			query := value[0]

			start := time.Now()
			expr, err := promql.ParseExpr(query)
			l.queryParseHist.Observe(time.Since(start).Seconds())
			if err != nil {
				return false, err
			}

			start = time.Now()
			labeled := l.AddLabels(expr, acl)
			l.labelerDurationHist.Observe(time.Since(start).Seconds())

			labeledQuery := labeled.String()
			(*params)[key] = []string{labeledQuery}
			modified = modified || query != labeledQuery
		}
	}
	return
}
