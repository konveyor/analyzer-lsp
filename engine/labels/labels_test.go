package labels

import (
	"testing"

	"github.com/konveyor/analyzer-lsp/engine/internal"
)

type ruleMeta struct {
	Labels []string
}

func (r ruleMeta) GetLabels() []string {
	return r.Labels
}

func Test_getBooleanExpression(t *testing.T) {
	tests := []struct {
		name          string
		expr          string
		compareLabels map[string][]string
		want          string
	}{
		{
			name: "complex expression 001",
			expr: "val && (konveyor.io/k1=20 && !konveyor.io/k2=30)",
			compareLabels: map[string][]string{
				"konveyor.io/k1": {"20"},
			},
			want: "false && ( true && ! false )",
		},
		{
			name: "complex expression 002",
			expr: "val && (konveyor.io/k1=20 && !konveyor.io/k2=30) || !val2",
			compareLabels: map[string][]string{
				"konveyor.io/k1": {"20"},
				"konveyor.io/k2": {"40"},
				"val2":           {""},
				"val":            {""},
			},
			want: "true && ( true && ! false ) || ! true",
		},
		{
			name: "complex expression 003",
			expr: "val && ((((((konveyor.io/k2=40)))))) || !val2",
			compareLabels: map[string][]string{
				"konveyor.io/k1": {"20"},
				"konveyor.io/k2": {"40"},
				"val2":           {""},
				"val":            {""},
			},
			want: "true && ( ( ( ( ( ( true ) ) ) ) ) ) || ! true",
		},
		{
			name: "duplicate keys 001",
			expr: "val && (konveyor.io/k2=40 || konveyor.io/k2=20)",
			compareLabels: map[string][]string{
				"konveyor.io/k1": {"20"},
				"konveyor.io/k2": {"40"},
				"val2":           {""},
				"val":            {""},
			},
			want: "true && ( true || false )",
		},
		{
			name: "duplicate keys 002",
			expr: "konveyor.io/k1=40 || (konveyor.io/k2=30 && konveyor.io/k2=40)",
			compareLabels: map[string][]string{
				"konveyor.io/k1": {"20"},
				"konveyor.io/k2": {"40", "30"},
			},
			want: "false || ( true && true )",
		},
		{
			name: "duplicate keys 003",
			expr: "(konveyor.io/k1=40 || konveyor.io/k1=10) || (konveyor.io/k2=30 && konveyor.io/k2=40)",
			compareLabels: map[string][]string{
				"konveyor.io/k1": {"20"},
				"konveyor.io/k2": {"40", "30"},
			},
			want: "( false || false ) || ( true && true )",
		},
		{
			name: "values with dots",
			expr: "(konveyor.io/target=eap8||konveyor.io/target=hibernate6.1)",
			compareLabels: map[string][]string{
				"konveyor.io/target": {"eap8", "hibernate6.1"},
			},
			want: "( true || true )",
		},
		{
			name: "values with spaces",
			expr: "(konveyor.io/fact=Spring   Beans  || konveyor.io/target=hibernate6.1)&& discovery && Label  With  Spaces",
			compareLabels: map[string][]string{
				"konveyor.io/target":  {"hibernate6.1"},
				"konveyor.io/fact":    {"Spring   Beans"},
				"Label  With  Spaces": {},
			},
			want: "( true || true ) && false && true",
		},
		{
			name: "values with version ranges",
			expr: "(konveyor.io/target=Spring Beans12  && konveyor.io/target=hibernate6.1)",
			compareLabels: map[string][]string{
				"konveyor.io/target": {"hibernate6-", "Spring Beans11+"},
			},
			want: "( true && false )",
		},
		{
			name: "values with version ranges",
			expr: "(konveyor.io/target=Spring Beans12  && konveyor.io/target=hibernate6.1)",
			compareLabels: map[string][]string{
				"konveyor.io/target": {"hibernate6+", "Spring Beans11-"},
			},
			want: "( false && true )",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getBooleanExpression(tt.expr, tt.compareLabels, matchesAny); got != tt.want {
				t.Errorf("getBooleanExpression() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseLabel(t *testing.T) {
	tests := []struct {
		name    string
		label   string
		wantKey string
		wantVal string
		wantErr bool
	}{
		{
			name:    "valid label 001",
			label:   "valid-label=",
			wantKey: "valid-label",
			wantVal: "",
		},
		{
			name:    "valid label 002",
			label:   "konveyor.io/valid-label",
			wantKey: "konveyor.io/valid-label",
			wantVal: "",
		},
		{
			name:    "valid label 003",
			label:   "dev.konveyor.io/sourceTech=tech10",
			wantKey: "dev.konveyor.io/sourceTech",
			wantVal: "tech10",
		},
		{
			name:    "valid label 004",
			label:   "dev.konveyor.io/sourceTech=",
			wantKey: "dev.konveyor.io/sourceTech",
			wantVal: "",
		},
		{
			name:    "invalid label 001",
			label:   "dev.konveyor#/sourceTech=tech10",
			wantErr: true,
		},
		{
			name:    "invalid label 002",
			label:   "dev.konveyor./sourceTech=tech10",
			wantErr: true,
		},
		{
			name:    "invalid label 003",
			label:   "dev.konveyor/",
			wantErr: true,
		},
		{
			name:    "dots in label values",
			label:   "konveyor.io/target=hibernate6.1",
			wantKey: "konveyor.io/target",
			wantVal: "hibernate6.1",
		},
		{
			name:    "spaces in label values",
			label:   "konveyor.io/fact=Spring Beans",
			wantKey: "konveyor.io/fact",
			wantVal: "Spring Beans",
		},
		{
			name:    "absolte version in value",
			label:   "konveyor.io/fact=Spring Beans12",
			wantKey: "konveyor.io/fact",
			wantVal: "Spring Beans12",
		},
		{
			name:    "version range + in value",
			label:   "konveyor.io/fact=Spring Beans12+",
			wantKey: "konveyor.io/fact",
			wantVal: "Spring Beans12+",
		},
		{
			name:    "version range - in value",
			label:   "konveyor.io/fact=Spring Beans12-",
			wantKey: "konveyor.io/fact",
			wantVal: "Spring Beans12-",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotKey, gotVal, err := ParseLabel(tt.label)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseRuleLabel() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotKey != tt.wantKey {
				t.Errorf("ParseRuleLabel() gotKey = %v, wantKey %v", gotKey, tt.wantKey)
			}
			if gotVal != tt.wantVal {
				t.Errorf("ParseRuleLabel() gotVal = %v, wantVal %v", gotVal, tt.wantVal)
			}
		})
	}
}

func TestNewRuleSelector(t *testing.T) {
	tests := []struct {
		name    string
		expr    string
		wantErr bool
	}{
		{
			name:    "valid expression 001",
			expr:    "prefix-less-value=",
			wantErr: false,
		},
		{
			name:    "valid expression 002",
			expr:    "((konveyor.io/sourceTech=eap7 && konveyor.io/targetTech=eap8) || !another-prefixless-val)",
			wantErr: false,
		},
		{
			name:    "valid expression 003",
			expr:    "!konveyor.io/sourceTech=golang && konveyor.io/targetTech=eap89 || ((a) && (a || konveyor.io/tech=30))",
			wantErr: false,
		},
		{
			name:    "valid expression 004",
			expr:    "!((((!(k1=v1) && (k2=v2) || konveyor.io/targetTech=java) && val || !val) && k2=v2) || !v3)",
			wantErr: false,
		},
		{
			name:    "invalid expression 001",
			expr:    "&&",
			wantErr: true,
		},
		{
			name:    "invalid expression 002",
			expr:    "a &&",
			wantErr: true,
		},
		{
			name:    "invalid expression 003",
			expr:    "k1=v1 || k2$$",
			wantErr: true,
		},
		{
			name:    "duplicate keys 001",
			expr:    "konveyor.io/source=go && konveyor.io/source=java",
			wantErr: false,
		},
		{
			name: "duplicate keys 002",
			expr: "(konveyor.io/source=java && konveyor.io/source=go) || (konveyor.io/target=java && konveyor.io/target=java)",
		},
		{
			name: "dots in label values",
			expr: "(konveyor.io/target=eap8.2.2||konveyor.io/target=hibernate6.1)",
		},
		{
			name: "spaces and dots in label values",
			expr: "konveyor.io/target=Spring     . Beans",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewLabelSelector[internal.VariableLabelSelector](tt.expr, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewRuleSelector() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}

func Test_ruleSelector_Matches(t *testing.T) {
	tests := []struct {
		name       string
		expr       string
		ruleLabels []string
		want       bool
	}{
		{
			name: "simple && query",
			expr: "konveyor.io/sourceTech=eap7 && konveyor.io/targetTech=eap10",
			ruleLabels: []string{
				"konveyor.io/sourceTech=eap7",
				"konveyor.io/targetTech=eap10",
			},
			want: true,
		},
		{
			name: "simple && query with !, no match",
			expr: "konveyor.io/sourceTech=eap7 && !konveyor.io/targetTech=eap10",
			ruleLabels: []string{
				"konveyor.io/sourceTech=eap7",
				"konveyor.io/targetTech=eap10",
			},
			want: false,
		},
		{
			name: "simple && query with !, match",
			expr: "konveyor.io/sourceTech=eap7 && !konveyor.io/targetTech=eap10",
			ruleLabels: []string{
				"konveyor.io/sourceTech=eap7",
				"konveyor.io/targetTech=eap11",
			},
			want: true,
		},
		{
			name: "nested && and || queries with !, match",
			expr: "(konveyor.io/sourceTech=eap7 && konveyor.io/targetTech=eap10) || (special-rule && !konveyor.io/type=restricted)",
			ruleLabels: []string{
				"konveyor.io/sourceTech=eap7",
				"konveyor.io/targetTech=eap11",
				"special-rule",
				"konveyor.io/type=special",
			},
			want: true,
		},
		{
			name: "nested && and || queries with mixed dots",
			expr: "(konveyor.io/sourceTech=eap7 && konveyor.io/targetTech=eap10) || (special-rule && !konveyor.io/type=restricted)",
			ruleLabels: []string{
				"konveyor.io/sourceTech=eap7",
				"konveyor.io/targetTech=eap11",
				"special-rule",
				"konveyor.io/type=special",
			},
			want: true,
		},
		{
			name: "nested && and || queries with mixed dots, unmatched",
			expr: "(konveyor.io/sourceTech=eap7 && konveyor.io/targetTech=eap10) || (!special-rule && !konveyor.io/type=restricted)",
			ruleLabels: []string{
				"konveyor.io/sourceTech=eap8",
				"konveyor.io/targetTech=eap11",
				"special-rule=diff-val",
				"konveyor.io/type=special",
			},
			want: false,
		},
		{
			name: "nested && and || queries with mixed dots, label values with spaces, matched",
			expr: "(konveyor.io/sourceTech=eap7 && konveyor.io/targetTech=eap10) || (!special-rule && !konveyor.io/type=restricted) ||konveyor.io/fact=Spring Beans",
			ruleLabels: []string{
				"konveyor.io/sourceTech=eap8",
				"konveyor.io/targetTech=eap11",
				"konveyor.io/fact=Spring Beans",
				"special-rule=diff-val",
				"konveyor.io/type=special",
			},
			want: true,
		},
		{
			name: "rule has a explicit include=always label",
			expr: "konveyor.io/source=test",
			ruleLabels: []string{
				"konveyor.io/include=always",
				"konveyor.io/source=noTest", // this should make the selector not match, but 'always' selector takes precedance
			},
			want: true,
		},
		{
			name: "rule has a explicit include=never label",
			expr: "konveyor.io/source=test",
			ruleLabels: []string{
				"konveyor.io/include=never",
				"konveyor.io/source=test", // this should make the selector match, but 'never' selector takes precedance
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _ := NewLabelSelector[Labeled](tt.expr, nil)
			if got, _ := s.Matches(ruleMeta{Labels: tt.ruleLabels}); got != tt.want {
				t.Errorf("ruleSelector.Matches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_labelValueMatches(t *testing.T) {
	tests := []struct {
		name      string
		candidate string
		matchWith string
		want      bool
	}{
		{
			name:      "no version range test",
			candidate: "eap",
			matchWith: "eap",
			want:      true,
		},
		{
			name:      "name mismatch test",
			candidate: "eap",
			matchWith: "javaee",
			want:      false,
		},
		{
			name:      "absolute version test",
			candidate: "eap6",
			matchWith: "eap6",
			want:      true,
		},
		{
			name:      "version range test for '+'",
			candidate: "eap6",
			matchWith: "eap5+",
			want:      true,
		},
		{
			name:      "version range test for '+'",
			candidate: "eap5",
			matchWith: "eap5+",
			want:      true,
		},
		{
			name:      "version range test for '-'",
			candidate: "eap7",
			matchWith: "eap8-",
			want:      true,
		},
		{
			name:      "version range negative test for '-'",
			candidate: "eap9",
			matchWith: "eap8-",
			want:      false,
		},
		{
			name:      "version range negative test for '+'",
			candidate: "eap7",
			matchWith: "eap8+",
			want:      false,
		},
		{
			name:      "complex value version range test",
			candidate: "Golang Version",
			matchWith: "Golang Version11+",
			want:      true,
		},
		{
			name:      "match any version test",
			candidate: "eap",
			matchWith: "eap6+",
			want:      true,
		},
		{
			name:      "match any version test negative",
			candidate: "eap6",
			matchWith: "eap",
			want:      false,
		},
		{
			name:      "float value absolute match",
			candidate: "hibernate5.1",
			matchWith: "hibernate5.1",
			want:      true,
		},
		{
			name:      "float value range symbol '+' match",
			candidate: "hibernate5.2",
			matchWith: "hibernate5.1+",
			want:      true,
		},
		{
			name:      "float value range symbol '+' negative match",
			candidate: "hibernate5.0.12",
			matchWith: "hibernate5.1+",
			want:      false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := labelValueMatches(tt.matchWith, tt.candidate); got != tt.want {
				t.Errorf("versionRangeMatches() = %v, want %v", got, tt.want)
			}
		})
	}
}
