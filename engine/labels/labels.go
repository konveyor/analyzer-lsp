package labels

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/PaesslerAG/gval"
)

const (
	LabelValueFmt  = "^[a-zA-Z0-9]([-a-zA-Z0-9.]*[a-zA-Z0-9])?$"
	LabelPrefixFmt = "^(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])?$"
)

type LabelSelector[T Labeled] struct {
	expr     string
	language gval.Language
}

func (l *LabelSelector[T]) Matches(v T) (bool, error) {
	ruleLabels, _ := ParseLabels(v.GetLabels())
	expr := getBooleanExpression(l.expr, ruleLabels)
	val, err := l.language.Evaluate(expr, nil)
	if err != nil {
		return false, err
	}
	if boolVal, ok := val.(bool); !ok {
		return false, nil
	} else {
		return boolVal, nil
	}
}

func (l *LabelSelector[T]) MatchList(list []T) ([]T, error) {
	newList := []T{}
	for _, v := range list {
		b, err := l.Matches(v)
		if err != nil {
			return nil, err
		}
		if b {
			i := &v
			newList = append(newList, *i)
		}
	}
	return newList, nil
}

type Labeled interface {
	GetLabels() []string
}

// NewRuleSelector returns a new rule selector that works on rule labels
// it enables using string expressions to form complex label queries
// supports "&&", "||" and "!" operators, "(" ")" for grouping, operands
// are string labels in key=val format, keys can be subdomain prefixed
func NewLabelSelector[T Labeled](expr string) (*LabelSelector[T], error) {
	language := gval.NewLanguage(
		gval.Ident(),
		gval.Parentheses(),
		gval.Constant("true", true),
		gval.Constant("false", false),
		gval.PrefixOperator("!", func(c context.Context, v interface{}) (interface{}, error) {
			b, ok := convertToBool(v)
			if !ok {
				return nil, fmt.Errorf("unexpected %T expected bool", v)
			}
			return !b, nil
		}),
		gval.InfixShortCircuit("&&", func(a interface{}) (interface{}, bool) { return false, a == false }),
		gval.InfixBoolOperator("&&", func(a, b bool) (interface{}, error) { return a && b, nil }),
		gval.InfixShortCircuit("||", func(a interface{}) (interface{}, bool) { return true, a == true }),
		gval.InfixBoolOperator("||", func(a, b bool) (interface{}, error) { return a || b, nil }),
	)
	// we need this hack to force validation
	_, err := gval.Evaluate(getBooleanExpression(expr, map[string][]string{}), nil)
	if err != nil {
		return nil, fmt.Errorf("invalid expression '%s'", expr)
	}
	return &LabelSelector[T]{
		language: language,
		expr:     expr,
	}, nil
}

// ParseLabels given a list of string labels, returns them as key=[val] map
// a list of values is needed because keys can be duplicate
func ParseLabels(labels []string) (map[string][]string, error) {
	keyVal := map[string][]string{}
	errors := []string{}
	for _, label := range labels {
		key, val, err := ParseLabel(label)
		if err != nil {
			errors = append(errors, err.Error())
			continue
		}
		if _, ok := keyVal[key]; !ok {
			keyVal[key] = []string{}
		}
		keyVal[key] = append(keyVal[key], val)
	}
	if len(errors) > 0 {
		return keyVal, fmt.Errorf("failed parsing, errors %v", errors)
	}
	return keyVal, nil
}

// ParseLabel given a string label converts into key=val
func ParseLabel(label string) (string, string, error) {
	valueRegex := regexp.MustCompile(LabelValueFmt)
	prefixRegex := regexp.MustCompile(LabelPrefixFmt)
	parts := strings.Split(label, "=")
	if len(parts) > 2 || len(parts) < 1 {
		return "", "", fmt.Errorf("invalid label '%s'", label)
	}
	key, val := parts[0], ""
	if len(parts) == 2 {
		if parts[1] != "" && !valueRegex.MatchString(parts[1]) {
			return "", "", fmt.Errorf("invalid label value '%s'", parts[1])
		}
		val = parts[1]
	}
	prefixParts := strings.Split(key, "/")
	if len(prefixParts) > 2 || len(prefixParts) < 1 {
		return "", "", fmt.Errorf("invalid label key '%s'", key)
	}
	if len(prefixParts) == 1 {
		if !valueRegex.MatchString(prefixParts[0]) {
			return "", "", fmt.Errorf("invalid label key '%s'", prefixParts[0])
		}
		return prefixParts[0], val, nil
	}
	if !prefixRegex.MatchString(prefixParts[0]) {
		return "", "", fmt.Errorf("invalid label key prefix '%s'", prefixParts[0])
	}
	if !valueRegex.MatchString(prefixParts[1]) {
		return "", "", fmt.Errorf("invalid label key suffix '%s'", prefixParts[1])
	}
	return key, val, nil
}

func convertToBool(o interface{}) (bool, bool) {
	if b, ok := o.(bool); ok {
		return b, true
	}
	if s, ok := o.(string); ok {
		switch s {
		case "true":
			return true, ok
		case "false":
			return false, ok
		}
	}
	return false, false
}

// getLabelsFromExpression parses only the labels from an expression
func getLabelsFromExpression(expr string) (map[string][]string, error) {
	labelsList := strings.FieldsFunc(expr, func(r rune) bool {
		return r == ' ' || r == '&' || r == '|' || r == '!' || r == '(' || r == ')'
	})
	labelsMap, err := ParseLabels(labelsList)
	if err != nil {
		return nil, err
	}
	return labelsMap, nil
}

// getBooleanExpression for every label in the string expression, check if the
// label key val is present in the given map and replace the label in the string
// expression with match result - true or false. we have to do this because gval
// does not understand labels as operands. "konveyor.io/k1=v1 && v2" will look
// something like "true && false" as a boolean expression depending on passed labels
// we wouldn't need this if gval supported writing custom operands
func getBooleanExpression(expr string, compareLabels map[string][]string) string {
	exprLabels, err := getLabelsFromExpression(expr)
	if err != nil {
		return expr
	}
	replaceMap := map[string]string{}
	for exprLabelKey, exprLabelVals := range exprLabels {
		for _, exprLabelVal := range exprLabelVals {
			toReplace := exprLabelKey
			if exprLabelVal != "" {
				toReplace = fmt.Sprintf("%s=%s", toReplace, exprLabelVal)
			}
			if labelVals, ok := compareLabels[exprLabelKey]; !ok {
				replaceMap[toReplace] = "false"
			} else if exprLabelVal != "" && !contains(exprLabelVal, labelVals) {
				replaceMap[toReplace] = "false"
			} else {
				replaceMap[toReplace] = "true"
			}
		}
	}
	expr = strings.ReplaceAll(expr, " ", "")
	tokens := regexp.MustCompile(`\s*(!|\|\||&&|\(|\)|[^!\s()&|]+)\s*`).FindAllString(expr, -1)
	expr = ""
	for _, token := range tokens {
		token = strings.TrimSuffix(token, "=")
		if val, ok := replaceMap[token]; ok {
			expr = fmt.Sprintf("%s %s", expr, val)
		} else {
			expr = fmt.Sprintf("%s %s", expr, token)
		}
	}
	expr = strings.Trim(expr, " ")
	return expr
}

func contains(elem string, items []string) bool {
	for _, item := range items {
		if item == elem {
			return true
		}
	}
	return false
}
