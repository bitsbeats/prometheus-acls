package labeler

import (
	"testing"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/promql"
)

var l = NewLabeler()

type aclMockAwesome struct{}

func (am aclMockAwesome) GetLabelMatchers(string) []*labels.Matcher {
	matchers, _ := ParseLabels("app=\"awesome\"")
	return matchers
}

type aclMockNone struct{}

func (am aclMockNone) GetLabelMatchers(string) []*labels.Matcher {
	return []*labels.Matcher{}
}

func TestParser(t *testing.T) {
	tests := []struct {
		input  string
		output string
		fail   bool
	}{{
		input:  "1",
		output: "1",
	}, {
		input:  "+Inf",
		output: "+Inf",
	}, {
		input:  "-Inf",
		output: "-Inf",
	}, {
		input:  ".5",
		output: "0.5",
	}, {
		input:  "5.",
		output: "5",
	}, {
		input:  "123.4567",
		output: "123.4567",
	}, {
		input:  "5e-3",
		output: "0.005",
	}, {
		input:  "5e3",
		output: "5000",
	}, {
		output: "12",
		input:  "0xc",
	}, {
		output: "493",
		input:  "0755",
	}, {
		output: "0.0055",
		input:  "+5.5e-3",
	}, {
		output: "-493",
		input:  "-0755",
	}, {
		output: "1 + 1",
		input:  "1 + 1",
	}, {
		output: "1 - 1",
		input:  "1 - 1",
	}, {
		output: "1 * 1",
		input:  "1 * 1",
	}, {
		output: "1 % 1",
		input:  "1 % 1",
	}, {
		output: "1 / 1",
		input:  "1 / 1",
	}, {
		output: "1 == bool 1",
		input:  "1 == bool 1",
	}, {
		output: "1 != bool 1",
		input:  "1 != bool 1",
	}, {
		output: "1 > bool 1",
		input:  "1 > bool 1",
	}, {
		output: "1 >= bool 1",
		input:  "1 >= bool 1",
	}, {
		output: "1 < bool 1",
		input:  "1 < bool 1",
	}, {
		output: "1 <= bool 1",
		input:  "1 <= bool 1",
	}, {
		output: "1 + -2 * 1",
		input:  "+1 + -2 * 1",
	}, {
		output: "1 + 2 / (3 * 1)",
		input:  "1 + 2/(3*1)",
	}, {
		output: "1 < bool 2 - 1 * 2",
		input:  "1 < bool 2 - 1 * 2",
	}, {
		output: "-some_metric{app=\"awesome\"}",
		input:  "-some_metric",
	}, {
		output: "+some_metric{app=\"awesome\"}",
		input:  "+some_metric",
	}, {
		input:  "1 and 1",
		output: "1 and 1",
		fail:   true,
	}, {
		input: "# just a comment\n\n",
		fail:  true,
	}, {
		input: "1+",
		fail:  true,
	}, {
		input: ".",
		fail:  true,
	}, {
		input: "2.5.",
		fail:  true,
	}, {
		input: "100..4",
		fail:  true,
	}, {
		input: "0deadbeef",
		fail:  true,
	}, {
		input: "1 /",
		fail:  true,
	}, {
		input: "*1",
		fail:  true,
	}, {
		input: "(1))",
		fail:  true,
	}, {
		input: "((1)",
		fail:  true,
	}, {
		input: "999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999",
		fail:  true,
	}, {
		input: "(",
		fail:  true,
	}, {
		input:  "1 and 1",
		output: "1 and 1",
		fail:   true,
	}, {
		input: "1 == 1",
		fail:  true,
	}, {
		input:  "1 or 1",
		output: "1 or 1",
		fail:   true,
	}, {
		input:  "1 unless 1",
		output: "1 unless 1",
		fail:   true,
	}, {
		input: "1 !~ 1",
		fail:  true,
	}, {
		input: "1 =~ 1",
		fail:  true,
	}, {
		input:  `-"string"`,
		output: `-"string"`,
		fail:   true,
	}, {
		input:  `-test[5m]`,
		output: `-test{app="awesome"}[5m]`,
		fail:   true,
	}, {
		input: `*test`,
		fail:  true,
	}, {
		input: "1 offset 1d",
		fail:  true,
	}, {
		input: "a - on(b) ignoring(c) d",
		fail:  true,
	}, {
		input:  "foo * bar",
		output: "foo{app=\"awesome\"} * bar{app=\"awesome\"}",
	}, {
		output: "foo{app=\"awesome\"} == 1",
		input:  "foo == 1",
	}, {
		output: "foo{app=\"awesome\"} == bool 1",
		input:  "foo == bool 1",
	}, {
		output: "2.5 / bar{app=\"awesome\"}",
		input:  "2.5 / bar",
	}, {
		output: "foo{app=\"awesome\"} and bar{app=\"awesome\"}",
		input:  "foo and bar",
	}, {
		output: "foo{app=\"awesome\"} or bar{app=\"awesome\"}",
		input:  "foo or bar",
	}, {
		output: "foo{app=\"awesome\"} unless bar{app=\"awesome\"}",
		input:  "foo unless bar",
	}, {
		output: "foo{app=\"awesome\"} + bar{app=\"awesome\"} or bla{app=\"awesome\"} and blub{app=\"awesome\"}",
		input:  "foo + bar or bla and blub",
	}, {
		output: "foo{app=\"awesome\"} and bar{app=\"awesome\"} unless baz{app=\"awesome\"} or qux{app=\"awesome\"}",
		input:  "foo and bar unless baz or qux",
	}, {
		output: "bar{app=\"awesome\"} + on(foo) bla{app=\"awesome\"} / on(baz, buz) group_right(test) blub{app=\"awesome\"}",
		input:  "bar + on(foo) bla / on(baz, buz) group_right(test) blub",
	}, {
		output: "foo{app=\"awesome\"} * on(test, blub) bar{app=\"awesome\"}",
		input:  "foo * on(test,blub) bar",
	}, {
		input:  "foo * on(test,blub) group_left bar",
		output: "foo{app=\"awesome\"} * on(test, blub) group_left() bar{app=\"awesome\"}",
	}, {
		output: "foo{app=\"awesome\"} and on(test, blub) bar{app=\"awesome\"}",
		input:  "foo and on(test,blub) bar",
	}, {
		output: "foo{app=\"awesome\"} and on() bar{app=\"awesome\"}",
		input:  "foo and on() bar",
	}, {
		output: "foo{app=\"awesome\"} and ignoring(test, blub) bar{app=\"awesome\"}",
		input:  "foo and ignoring(test,blub) bar",
	}, {
		input:  "foo and ignoring() bar",
		output: "foo{app=\"awesome\"} and bar{app=\"awesome\"}",
	}, {
		output: "foo{app=\"awesome\"} unless on(bar) baz{app=\"awesome\"}",
		input:  "foo unless on(bar) baz",
	}, {
		output: "foo{app=\"awesome\"} / on(test, blub) group_left(bar) bar{app=\"awesome\"}",
		input:  "foo / on(test,blub) group_left(bar) bar",
	}, {
		output: "foo{app=\"awesome\"} / ignoring(test, blub) group_left(blub) bar{app=\"awesome\"}",
		input:  "foo / ignoring(test,blub) group_left(blub) bar",
	}, {
		output: "foo{app=\"awesome\"} / ignoring(test, blub) group_left(bar) bar{app=\"awesome\"}",
		input:  "foo / ignoring(test,blub) group_left(bar) bar",
	}, {
		output: "foo{app=\"awesome\"} - on(test, blub) group_right(bar, foo) bar{app=\"awesome\"}",
		input:  "foo - on(test,blub) group_right(bar,foo) bar",
	}, {
		output: "foo{app=\"awesome\"} - ignoring(test, blub) group_right(bar, foo) bar{app=\"awesome\"}",
		input:  "foo - ignoring(test,blub) group_right(bar,foo) bar",
	}, {
		output: "foo{app=\"awesome\"} and 1",
		input:  "foo and 1",
		fail:   true,
	}, {
		input:  "1 and foo",
		output: "1 and foo{app=\"awesome\"}",
		fail:   true,
	}, {
		output: "foo{app=\"awesome\"} or 1",
		input:  "foo or 1",
		fail:   true,
	}, {
		input:  "1 or foo",
		output: "1 or foo{app=\"awesome\"}",
		fail:   true,
	}, {
		output: "foo{app=\"awesome\"} unless 1",
		input:  "foo unless 1",
		fail:   true,
	}, {
		output: "1 unless foo{app=\"awesome\"}",
		input:  "1 unless foo",
		fail:   true,
	}, {
		input:  "1 or on(bar) foo",
		output: "1 or on(bar) foo{app=\"awesome\"}",
		fail:   true,
	}, {
		input:  "foo == on(bar) 10",
		output: "foo{app=\"awesome\"} == on(bar) 10",
		fail:   true,
	}, {
		input:  "foo and on(bar) group_left(baz) bar",
		output: "foo{app=\"awesome\"} and on(bar) group_left(baz) bar{app=\"awesome\"}",
		fail:   true,
	}, {
		output: "foo{app=\"awesome\"} and on(bar) group_right(baz) bar{app=\"awesome\"}",
		input:  "foo and on(bar) group_right(baz) bar",
		fail:   true,
	}, {
		output: "foo{app=\"awesome\"} or on(bar) group_left(baz) bar{app=\"awesome\"}",
		input:  "foo or on(bar) group_left(baz) bar",
		fail:   true,
	}, {
		output: "foo{app=\"awesome\"} or on(bar) group_right(baz) bar{app=\"awesome\"}",
		input:  "foo or on(bar) group_right(baz) bar",
		fail:   true,
	}, {
		input:  "foo unless on(bar) group_left(baz) bar",
		output: "foo{app=\"awesome\"} unless on(bar) group_left(baz) bar{app=\"awesome\"}",
		fail:   true,
	}, {
		input:  "foo unless on(bar) group_right(baz) bar",
		output: "foo{app=\"awesome\"} unless on(bar) group_right(baz) bar{app=\"awesome\"}",
		fail:   true,
	}, {
		input: `http_request}, {group="production"} + on(instance) group_left(job,instance) cpu_count{type="smp"}`,
		fail:  true,
	}, {
		input: "foo + bool bar",
		fail:  true,
	}, {
		input: "foo + bool 10",
		fail:  true,
	}, {
		input: "foo and bool 10",
		fail:  true,
	}, {
		output: "foo{app=\"awesome\"}",
		input:  "foo",
	}, {
		output: "foo{app=\"awesome\"} offset 5m",
		input:  "foo offset 5m",
	}, {
		output: "foo:bar{a=\"bc\",app=\"awesome\"}",
		input:  `foo:bar{a="bc"}`,
	}, {
		output: "foo{NaN=\"bc\",app=\"awesome\"}",
		input:  `foo{NaN='bc'}`,
	}, {
		output: "foo{a=\"b\",app=\"awesome\",bar!~\"baz\",foo!=\"bar\",test=~\"test\"}",
		input:  `foo{a="b", foo!="bar", test=~"test", bar!~"baz"}`,
	}, {
		input: `{`,
		fail:  true,
	}, {
		input: `}`,
		fail:  true,
	}, {
		input: `some{`,
		fail:  true,
	}, {
		input: `some}`,
		fail:  true,
	}, {
		input: `some_metric{a=b}`,
		fail:  true,
	}, {
		input: `some_metric{a:b="b"}`,
		fail:  true,
	}, {
		input: `foo{a*"b"}`,
		fail:  true,
	}, {
		input: `foo{a>="b"}`,
		fail:  true,
	}, {
		input: "some_metric{a=\"\xff\"}",
		fail:  true,
	}, {
		input: `foo{gibberish}`,
		fail:  true,
	}, {
		input: `foo{1}`,
		fail:  true,
	}, {
		input: `{}`,
		fail:  true,
	}, {
		input: `{x=""}`,
		fail:  true,
	}, {
		input: `{x=~".*"}`,
		fail:  true,
	}, {
		input: `{x!~".+"}`,
		fail:  true,
	}, {
		input: `{x!="a"}`,
		fail:  true,
	}, {
		input: `foo{__name__="bar"}`,
		fail:  true,
	}, {
		output: "test{app=\"awesome\"}[5s]",
		input:  "test[5s]",
	}, {
		output: "test{app=\"awesome\"}[5m]",
		input:  "test[5m]",
	}, {
		output: "test{app=\"awesome\"}[5h] offset 5m",
		input:  "test[5h] OFFSET 5m",
	}, {
		output: "test{app=\"awesome\"}[5d] offset 10s",
		input:  "test[5d] OFFSET 10s",
	}, {
		output: "test{app=\"awesome\"}[5w] offset 2w",
		input:  "test[5w] offset 2w",
	}, {
		output: "test{a=\"b\",app=\"awesome\"}[5y] offset 3d",
		input:  `test{a="b"}[5y] OFFSET 3d`,
	}, {
		input: `foo[5mm]`,
		fail:  true,
	}, {
		input: `foo[0m]`,
		fail:  true,
	}, {
		input: `foo[5m30s]`,
		fail:  true,
	}, {
		input: `foo[5m] OFFSET 1h30m`,
		fail:  true,
	}, {
		input: `foo["5m"]`,
		fail:  true,
	}, {
		input: `foo[]`,
		fail:  true,
	}, {
		input: `foo[1]`,
		fail:  true,
	}, {
		input: `some_metric[5m] OFFSET 1`,
		fail:  true,
	}, {
		input: `some_metric[5m] OFFSET 1mm`,
		fail:  true,
	}, {
		input: `some_metric[5m] OFFSET`,
		fail:  true,
	}, {
		input: `some_metric OFFSET 1m[5m]`,
		fail:  true,
	}, {
		input: `(foo + bar)[5m]`,
		fail:  true,
	}, {
		output: "sum by(foo) (some_metric{app=\"awesome\"})",
		input:  "sum by (foo)(some_metric)",
	}, {
		output: "avg by(foo) (some_metric{app=\"awesome\"})",
		input:  "avg by (foo)(some_metric)",
	}, {
		output: "max by(foo) (some_metric{app=\"awesome\"})",
		input:  "max by (foo)(some_metric)",
	}, {
		output: "sum without(foo) (some_metric{app=\"awesome\"})",
		input:  "sum without (foo) (some_metric)",
	}, {
		output: "sum without(foo) (some_metric{app=\"awesome\"})",
		input:  "sum (some_metric) without (foo)",
	}, {
		output: "stddev(some_metric{app=\"awesome\"})",
		input:  "stddev(some_metric)",
	}, {
		output: "stdvar by(foo) (some_metric{app=\"awesome\"})",
		input:  "stdvar by (foo)(some_metric)",
	}, {
		output: "sum(some_metric{app=\"awesome\"})",
		input:  "sum by ()(some_metric)",
	}, {
		output: "topk(5, some_metric{app=\"awesome\"})",
		input:  "topk(5, some_metric)",
	}, {
		output: "count_values(\"value\", some_metric{app=\"awesome\"})",
		input:  "count_values(\"value\", some_metric)",
	}, {
		output: "sum without(and, by, avg, count, alert, annotations) (some_metric{app=\"awesome\"})",
		input:  "sum without(and, by, avg, count, alert, annotations)(some_metric)",
	}, {
		input: "sum without(==)(some_metric)",
		fail:  true,
	}, {
		input: `sum some_metric by (test)`,
		fail:  true,
	}, {
		input: `sum (some_metric) by test`,
		fail:  true,
	}, {
		input: `sum (some_metric) by test`,
		fail:  true,
	}, {
		input: `sum () by (test)`,
		fail:  true,
	}, {
		input: "MIN keep_common (some_metric)",
		fail:  true,
	}, {
		input: "MIN (some_metric) keep_common",
		fail:  true,
	}, {
		input: `sum (some_metric) without (test) by (test)`,
		fail:  true,
	}, {
		input: `sum without (test) (some_metric) by (test)`,
		fail:  true,
	}, {
		input:  `topk(some_metric)`,
		output: `topk(some_metric{app=\"awesome\"})`,
		fail:   true,
	}, {
		input:  `topk(some_metric, other_metric)`,
		output: "topk(some_metric, other_metric{app=\"awesome\"})",
		fail:   true,
	}, {
		input:  `count_values(5, other_metric)`,
		output: "count_values(5, other_metric{app=\"awesome\"})",
		fail:   true,
	}, {
		output: "time()",
		input:  "time()",
	}, {
		output: "floor(some_metric{app=\"awesome\",foo!=\"bar\"})",
		input:  `floor(some_metric{foo!="bar"})`,
	}, {
		output: "rate(some_metric{app=\"awesome\"}[5m])",
		input:  "rate(some_metric[5m])",
	}, {
		output: "round(some_metric{app=\"awesome\"})",
		input:  "round(some_metric)",
	}, {
		output: "round(some_metric{app=\"awesome\"}, 5)",
		input:  "round(some_metric, 5)",
	}, {
		output: "floor()",
		input:  "floor()",
		fail:   true,
	}, {
		output: "floor(some_metric{app=\"awesome\"}, other_metric{app=\"awesome\"})",
		input:  "floor(some_metric, other_metric)",
		fail:   true,
	}, {
		input:  "floor(1)",
		output: "floor(1)",
		fail:   true,
	}, {
		input: "non_existent_function_far_bar()",
		fail:  true,
	}, {
		input:  "rate(some_metric)",
		output: "rate(some_metric{app=\"awesome\"})",
		fail:   true,
	}, {
		input: "label_replace(a, `b`, `c\xff`, `d`, `.*`)",
		fail:  true,
	}, {
		input: "-=",
		fail:  true,
	}, {
		input: "++-++-+-+-<",
		fail:  true,
	}, {
		input: "e-+=/(0)",
		fail:  true,
	}, {
		output: "\"double-quoted string \\\" with escaped quote\"",
		input:  `"double-quoted string \" with escaped quote"`,
	}, {
		output: "\"single-quoted string ' with escaped quote\"",
		input:  `'single-quoted string \' with escaped quote'`,
	}, {
		output: "\"backtick-quoted string\"",
		input:  "`backtick-quoted string`",
	}, {
		output: "\"\\a\\b\\f\\n\\r\\t\\v\\\\\\\" - \\xff\\xffሴ𐄑𐄑11☺\"",
		input:  `"\a\b\f\n\r\t\v\\\" - \xFF\377\u1234\U00010111\U0001011111☺"`,
	}, {
		output: "\"\\a\\b\\f\\n\\r\\t\\v\\\\' - \\xff\\xffሴ𐄑𐄑11☺\"",
		input:  `'\a\b\f\n\r\t\v\\\' - \xFF\377\u1234\U00010111\U0001011111☺'`,
	}, {
		output: "\"\\\\a\\\\b\\\\f\\\\n\\\\r\\\\t\\\\v\\\\\\\\\\\\\\\"\\\\' - \\\\xFF\\\\377\\\\u1234\\\\U00010111\\\\U0001011111☺\"",
		input:  "`" + `\a\b\f\n\r\t\v\\\"\' - \xFF\377\u1234\U00010111\U0001011111☺` + "`",
	}, {
		input: "`\\``",
		fail:  true,
	}, {
		input: `"\`,
		fail:  true,
	}, {
		input: `"\c"`,
		fail:  true,
	}, {
		input: `"\x."`,
		fail:  true,
	}, {
		output: "foo{app=\"awesome\",bar=\"baz\"}[10m:6s]",
		input:  `foo{bar="baz"}[10m:6s]`,
	}, {
		output: "foo{app=\"awesome\"}[10m:]",
		input:  `foo[10m:]`,
	}, {
		output: "min_over_time(rate(foo{app=\"awesome\",bar=\"baz\"}[2s])[5m:5s])",
		input:  `min_over_time(rate(foo{bar="baz"}[2s])[5m:5s])`,
	}, {
		output: "min_over_time(rate(foo{app=\"awesome\",bar=\"baz\"}[2s])[5m:])[4m:3s]",
		input:  `min_over_time(rate(foo{bar="baz"}[2s])[5m:])[4m:3s]`,
	}, {
		output: "min_over_time(rate(foo{app=\"awesome\",bar=\"baz\"}[2s])[5m:])[4m:3s]",
		input:  `min_over_time(rate(foo{bar="baz"}[2s])[5m:] offset 4m)[4m:3s]`,
	}, {
		output: "sum without(and, by, avg, count, alert, annotations) (some_metric{app=\"awesome\"})[30m:10s]",
		input:  "sum without(and, by, avg, count, alert, annotations)(some_metric) [30m:10s]",
	}, {
		output: "some_metric{app=\"awesome\"} offset 1m[10m:5s]",
		input:  `some_metric OFFSET 1m [10m:5s]`,
	}, {
		input:  `(foo + bar{nm="val"})[5m:]`,
		output: "(foo{app=\"awesome\"} + bar{app=\"awesome\",nm=\"val\"})[5m:]",
	}, {
		input:  `(foo + bar{nm="val"})[5m:] offset 10m`,
		output: `(foo{app="awesome"} + bar{app="awesome",nm="val"})[5m:]`,
	}, {
		input:  "test[5d] OFFSET 10s [10m:5s]",
		output: "test{app=\"awesome\"}[5d] offset 10s[10m:5s]",
		fail:   true,
	}, {
		input: `(foo + bar{nm="val"})[5m:][10m:5s]`,
		fail:  true,
	}}
	for _, test := range tests {
		parsed, err := promql.ParseExpr(test.input)
		if err != nil && !test.fail {
			t.Fatalf("should not fail: %s", test.input)
		}
		if parsed != nil {
			labeled := l.AddLabels(parsed, aclMockAwesome{})
			if test.output != labeled.String() {
				t.Fatalf(
					"invalid return:\nin:  %s\nwant: %s\ngot:  %s",
					test.input,
					test.output,
					labeled.String(),
				)
			}
		}
	}
}

func TestDedupe(t *testing.T) {
	tests := []struct {
		input  string
		output string
	}{
		// != tests
		{
			input:  "up{name!=\"hello\",name!=\"hello\"}",
			output: "up{name!=\"hello\"}",
		}, {
			input:  "up{name!=\"hello\",name=\"hello2\"}",
			output: "up{name=\"hello2\"}",
		}, {
			input:  "up{name!=\"hello\",name!=\"hello2\"}",
			output: "up{name!=\"hello\",name!=\"hello2\"}",
		},
		// = tests
		{
			input:  "up{name=\"hello\",name=\"hello\"}",
			output: "up{name=\"hello\"}",
		}, {
			input:  "up{name=\"hello\",name=\"hello2\"}",
			output: "up{__=\"none\"}",
		},
		// !~ tests
		{
			input:  "up{name!~\"hel.*\",name!~\"hel.*\"}",
			output: "up{name!~\"hel.*\"}",
		}, {
			input:  "up{name!~\"hel.*\",name!~\"hell.*\"}",
			output: "up{name!~\"hel.*\",name!~\"hell.*\"}",
		}, {
			input:  "up{name!~\"hel.*\",name=\"hello\"}",
			output: "up{__=\"none\"}",
		}, {
			input:  "up{name!~\"hel.*\",name=\"foo\"}",
			output: "up{name=\"foo\"}",
		},
		// =~ tests
		{
			input:  "up{name=\"hello\",name=~\"hel.*\"}",
			output: "up{name=\"hello\"}",
		}, {
			input:  "up{name=\"hello\",name=~\"xhel.*\"}",
			output: "up{__=\"none\"}",
		}, {
			input:  "up{name=\"hello\",nami=~\"hel.*\"}",
			output: "up{name=\"hello\",nami=~\"hel.*\"}",
		},
	}
	for _, test := range tests {
		parsed, _ := promql.ParseExpr(test.input)
		parsed = l.AddLabels(parsed, aclMockNone{})
		if got := parsed.String(); got != test.output {
			t.Fatalf("invalid return:\nin:  %s\nwant: %s\ngot:  %s",
				test.input,
				test.output,
				got,
			)
		}
	}
}
