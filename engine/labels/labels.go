package labels

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/PaesslerAG/gval"
	"github.com/hashicorp/go-version"
)

const (
	// a selector label takes precedance over any other label when matching
	RuleIncludeLabel = "konveyor.io/include"
	SelectAlways     = "always"
	SelectNever      = "never"
)

const (
	LabelValueFmt      = "^[a-zA-Z0-9]([-a-zA-Z0-9. ]*[a-zA-Z0-9+-])?$"
	LabelPrefixFmt     = "^(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])?$"
	exprSpecialSymbols = `!|\|\||&&|\(|\)`
	// used to split string into groups of special symbols and everything else
	exprSplitter = `(` + exprSpecialSymbols + `|[^!` + exprSpecialSymbols + `]+)`
)

type LabelSelector[T Labeled] struct {
	expr     string
	language gval.Language
	matchAny MatchAny
}

// Helper function to refactor key value label manipulation
func AsString(key, value string) string {
	if value == "" {
		return fmt.Sprintf("%s", key)
	}
	return fmt.Sprintf("%s=%s", key, value)
}

func (l *LabelSelector[T]) Matches(v T) (bool, error) {
	ruleLabels, _ := ParseLabels(v.GetLabels())
	if val, ok := ruleLabels[RuleIncludeLabel]; ok &&
		val != nil && len(val) > 0 {
		switch val[0] {
		case SelectAlways:
			return true, nil
		case SelectNever:
			return false, nil
		}
	}
	expr := getBooleanExpression(l.expr, ruleLabels, l.matchAny)
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

type MatchAny func(elem string, items []string) bool

// NewRuleSelector returns a new rule selector that works on rule labels
// it enables using string expressions to form complex label queries
// supports "&&", "||" and "!" operators, "(" ")" for grouping, operands
// are string labels in key=val format, keys can be subdomain prefixed
func NewLabelSelector[T Labeled](expr string, match MatchAny) (*LabelSelector[T], error) {
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
	_, err := gval.Evaluate(getBooleanExpression(expr, map[string][]string{}, matchesAny), nil)
	if err != nil {
		return nil, fmt.Errorf("invalid expression '%s'", expr)
	}
	if match == nil {
		match = matchesAny
	}
	return &LabelSelector[T]{
		language: language,
		expr:     expr,
		matchAny: match,
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
	labelsList := []string{}
	for _, token := range tokenize(expr) {
		if token == "" ||
			regexp.MustCompile(exprSpecialSymbols).MatchString(token) {
			continue
		}
		labelsList = append(labelsList, token)
	}
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
func getBooleanExpression(expr string, compareLabels map[string][]string, matchAny MatchAny) string {
	exprLabels, err := getLabelsFromExpression(expr)
	if err != nil {
		return expr
	}
	replaceMap := map[string]string{}
	for exprLabelKey, exprLabelVals := range exprLabels {
		for _, exprLabelVal := range exprLabelVals {
			toReplace := exprLabelKey
			if exprLabelVal != "" {
				toReplace = AsString(toReplace, exprLabelVal)
			}
			if labelVals, ok := compareLabels[exprLabelKey]; !ok {
				replaceMap[toReplace] = "false"
			} else if exprLabelVal != "" && !matchAny(exprLabelVal, labelVals) {
				replaceMap[toReplace] = "false"
			} else {
				replaceMap[toReplace] = "true"
			}
		}
	}
	boolExpr := ""
	for _, token := range tokenize(expr) {
		if val, ok := replaceMap[token]; ok {
			boolExpr = fmt.Sprintf("%s %s", boolExpr, val)
		} else {
			boolExpr = fmt.Sprintf("%s %s", boolExpr, token)
		}
	}
	boolExpr = strings.Trim(boolExpr, " ")
	return boolExpr
}

func tokenize(expr string) []string {
	tokens := []string{}
	for _, token := range regexp.MustCompile(exprSplitter).FindAllString(expr, -1) {
		token = strings.Trim(token, " ")
		token = strings.TrimSuffix(token, "=")
		if token != "" {
			tokens = append(tokens, token)
		}
	}
	return tokens
}

func matchesAny(elem string, items []string) bool {
	for _, item := range items {
		if labelValueMatches(item, elem) {
			return true
		}
	}
	return false
}

// labelValueMatches returns true when candidate matches with matchWith
// label value is divided into two parts - name and version
// version is absolute version or a range denoted by + or -
// returns true when names of values are equal and the version of
// candidate falls within the version range of matchWith
func labelValueMatches(matchWith string, candidate string) bool {
	versionRegex := regexp.MustCompile(`(\d(?:[\d\.]*\d)?)([\+-])?$`)
	mMatch := versionRegex.FindStringSubmatch(matchWith)
	cMatch := versionRegex.FindStringSubmatch(candidate)
	if len(mMatch) != 3 {
		return candidate == matchWith
	}
	mName, mVersion, mVersionRangeSymbol :=
		versionRegex.ReplaceAllString(matchWith, ""), mMatch[1], mMatch[2]
	if len(cMatch) != 3 {
		// when no version on candidate, match for any version
		return mName == candidate
	}
	cName, cVersion :=
		versionRegex.ReplaceAllString(candidate, ""), cMatch[1]
	if mName != cName {
		return false
	}
	if mVersion == "" {
		return mVersion == cVersion
	}
	if cVersion == "" {
		return true
	}
	cSemver, err := version.NewSemver(cVersion)
	if err != nil {
		return cVersion == mVersion
	}
	mSemver, err := version.NewSemver(mVersion)
	if err != nil {
		return cVersion == mVersion
	}
	switch mVersionRangeSymbol {
	case "+":
		return cSemver.GreaterThanOrEqual(mSemver)
	case "-":
		return mSemver.GreaterThanOrEqual(cSemver)
	default:
		return cSemver.Equal(mSemver)
	}
}
