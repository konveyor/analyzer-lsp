package labels

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/PaesslerAG/gval"
	"github.com/konveyor/analyzer-lsp/engine"
)

const (
	LabelValueFmt  = "^[a-zA-Z0-9]([-a-zA-Z0-9]*[a-zA-Z0-9])?$"
	LabelPrefixFmt = "^(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])?$"
)

// ruleSelector is a label selector
type ruleSelector struct {
	expr     string
	language gval.Language
}

// Matches returns true when given rule labels match with expression
func (s ruleSelector) Matches(m engine.RuleMeta) bool {
	ruleLabels, _ := ParseLabels(m.Labels)
	expr := getBooleanExpression(s.expr, ruleLabels)
	val, err := s.language.Evaluate(expr, nil)
	if err != nil {
		return false
	}
	if boolVal, ok := val.(bool); !ok {
		return false
	} else {
		return boolVal
	}
}

// NewRuleSelector returns a new rule selector that works on rule labels
// it enables using string expressions to form complex label queries
// supports "&&", "||" and "!" operators, "(" ")" for grouping, operands
// are string labels in key=val format, keys can be subdomain prefixed
func NewRuleSelector(expr string) (engine.RuleSelector, error) {
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
	_, err := gval.Evaluate(getBooleanExpression(expr, map[string]string{}), nil)
	if err != nil {
		return nil, fmt.Errorf("invalid expression '%s'", expr)
	}
	return ruleSelector{
		language: language,
		expr:     expr,
	}, nil
}

// ParseLabels given a list of string labels, returns them as key=val map
func ParseLabels(labels []string) (map[string]string, error) {
	keyVal := map[string]string{}
	errors := []string{}
	for _, label := range labels {
		key, val, err := ParseLabel(label)
		if err != nil {
			errors = append(errors, err.Error())
			continue
		}
		keyVal[key] = val
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
func getLabelsFromExpression(expr string) (map[string]string, error) {
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
func getBooleanExpression(expr string, labels map[string]string) string {
	exprLabels, err := getLabelsFromExpression(expr)
	if err != nil {
		return expr
	}
	replaceMap := map[string]string{}
	for exprLabelKey, exprLabelVal := range exprLabels {
		toReplace := exprLabelKey
		if exprLabelVal != "" {
			toReplace = fmt.Sprintf("%s=%s", toReplace, exprLabelVal)
		}
		if labelVal, ok := labels[exprLabelKey]; !ok {
			replaceMap[toReplace] = "false"
		} else if exprLabelVal != "" && labelVal != exprLabelVal {
			replaceMap[toReplace] = "false"
		} else {
			replaceMap[toReplace] = "true"
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
