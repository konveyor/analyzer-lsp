package nodejs_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/konveyor/analyzer-lsp/external-providers/generic-external-provider/pkg/server_configurations/nodejs"
)

// Test normalizeMultilineImports
func TestNormalizeMultilineImports(t *testing.T) {
	sc := &nodejs.NodeServiceClient{}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "Single line import unchanged",
			input: `import { Button } from '@patternfly/react-core';
console.log('test');`,
			expected: `import { Button } from '@patternfly/react-core';
console.log('test');`,
		},
		{
			name: "Multiline import normalized",
			input: `import {
  Button,
  Card
} from '@patternfly/react-core';`,
			expected: `import {   Button,   Card } from '@patternfly/react-core';`,
		},
		{
			name: "Nested braces in import preserved",
			input: `import {
  type Foo,
  Button
} from '@patternfly/react-core';`,
			expected: `import {   type Foo,   Button } from '@patternfly/react-core';`,
		},
		{
			name: "String literals with newlines preserved",
			input: `const x = "line1\nline2";
import { Button } from 'pkg';`,
			expected: `const x = "line1\nline2";
import { Button } from 'pkg';`,
		},
		{
			name: "Comment handling",
			input: `import {
  Button, // Main button
  Card    // Card component
} from '@patternfly/react-core';`,
			expected: `import {   Button, // Main button   Card    // Card component } from '@patternfly/react-core';`,
		},
		{
			name: "Windows line endings (CRLF) normalized",
			input: "import {\r\n  Button,\r\n  Card\r\n} from 'pkg';",
			expected: `import {   Button,   Card } from 'pkg';`,
		},
		{
			name: "Mixed line endings normalized",
			input: "import {\r\n  Button,\n  Card\r\n} from 'pkg';",
			expected: `import {   Button,   Card } from 'pkg';`,
		},
		{
			name: "Escaped quotes in import string",
			input: `import { Button } from "my\"quoted\"package";`,
			expected: `import { Button } from "my\"quoted\"package";`,
		},
		{
			name: "Double backslash before quote",
			input: `import { Button } from "my\\\"path";`,
			expected: `import { Button } from "my\\\"path";`,
		},
		{
			name: "Template literals with backticks",
			input: "import { Button } from `@patternfly/react-core`;",
			expected: "import { Button } from `@patternfly/react-core`;",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sc.NormalizeMultilineImportsPublic(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeMultilineImports() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// Test findImportStatements with various import patterns
func TestFindImportStatements(t *testing.T) {
	// Create temporary test files
	tmpDir := t.TempDir()

	testCases := []struct {
		name           string
		fileContent    string
		pattern        string
		expectedCount  int
		expectedLine   uint32
		expectedColumn uint32
	}{
		{
			name: "Named import - simple",
			fileContent: `import { Button } from '@patternfly/react-core';
export const MyButton = Button;`,
			pattern:        "Button",
			expectedCount:  1,
			expectedLine:   0,
			expectedColumn: 9,
		},
		{
			name: "Named import - multiple",
			fileContent: `import { Button, Card, Chip } from '@patternfly/react-core';
export const MyButton = Button;`,
			pattern:        "Card",
			expectedCount:  1,
			expectedLine:   0,
			expectedColumn: 17,
		},
		{
			name: "Multiline named import",
			fileContent: `import {
  Button,
  Card,
  Chip
} from '@patternfly/react-core';`,
			pattern:        "Card",
			expectedCount:  1,
			expectedLine:   2,
			expectedColumn: 2,
		},
		{
			name: "Default import",
			fileContent: `import React from 'react';
export const Component = () => <div />;`,
			pattern:        "React",
			expectedCount:  1,
			expectedLine:   0,
			expectedColumn: 7,
		},
		{
			name: "Pattern not found",
			fileContent: `import { Button } from '@patternfly/react-core';
export const MyButton = Button;`,
			pattern:       "Card",
			expectedCount: 0,
		},
		{
			name: "Word boundary - avoid partial match",
			fileContent: `import { Card, CardBody } from '@patternfly/react-core';
export const MyCard = Card;`,
			pattern:        "Card",
			expectedCount:  1,
			expectedLine:   0,
			expectedColumn: 9,
		},
		{
			name: "TypeScript type import - named",
			fileContent: `import type { Button } from '@patternfly/react-core';
export const MyButton = Button;`,
			pattern:        "Button",
			expectedCount:  1,
			expectedLine:   0,
			expectedColumn: 14,
		},
		{
			name: "Mixed imports",
			fileContent: `import React from 'react';
import { Button, Card } from '@patternfly/react-core';
import { useState } from 'react';`,
			pattern:        "Button",
			expectedCount:  1,
			expectedLine:   1,
			expectedColumn: 9,
		},
		{
			name: "Namespace import",
			fileContent: `import * as PatternFly from '@patternfly/react-core';
export const MyCard = PatternFly.Card;`,
			pattern:        "PatternFly",
			expectedCount:  1,
			expectedLine:   0,
			expectedColumn: 12,
		},
		{
			name: "Namespace import - multiline",
			fileContent: `import * as
PatternFly
from '@patternfly/react-core';`,
			pattern:        "PatternFly",
			expectedCount:  1,
			expectedLine:   1,
			expectedColumn: 0,
		},
		{
			name: "Default + named import (mixed)",
			fileContent: `import React, { useState, useEffect } from 'react';
export const Component = () => {};`,
			pattern:        "React",
			expectedCount:  1,
			expectedLine:   0,
			expectedColumn: 7,
		},
		{
			name: "Default + named import - search named",
			fileContent: `import React, { useState, useEffect } from 'react';
export const Component = () => {};`,
			pattern:        "useState",
			expectedCount:  1,
			expectedLine:   0,
			expectedColumn: 16,
		},
		{
			name: "Default + namespace import (rare)",
			fileContent: `import React, * as ReactAll from 'react';
export const Component = () => {};`,
			pattern:        "ReactAll",
			expectedCount:  1,
			expectedLine:   0,
			expectedColumn: 19,
		},
		{
			name: "Side-effect import - no symbols",
			fileContent: `import '@patternfly/react-core/dist/styles/base.css';
export const Component = () => {};`,
			pattern:       "Card",
			expectedCount: 0,
		},
		{
			name: "Namespace import - pattern not found",
			fileContent: `import * as PatternFly from '@patternfly/react-core';
export const MyCard = PatternFly.Card;`,
			pattern:       "Card",
			expectedCount: 0, // "Card" is not the namespace identifier
		},
		{
			name: "Default + named - search default part",
			fileContent: `import React, { useState } from 'react';
const x = useState();`,
			pattern:        "React",
			expectedCount:  1,
			expectedLine:   0,
			expectedColumn: 7,
		},
		{
			name: "Default + namespace - search default part",
			fileContent: `import React, * as ReactAll from 'react';
const x = ReactAll.useState();`,
			pattern:        "React",
			expectedCount:  1,
			expectedLine:   0,
			expectedColumn: 7,
		},
		{
			name: "Multiline default + named",
			fileContent: `import React, {
  useState,
  useEffect
} from 'react';`,
			pattern:        "useEffect",
			expectedCount:  1,
			expectedLine:   2,
			expectedColumn: 2,
		},
		{
			name: "Multiple namespace imports in file",
			fileContent: `import * as PF from '@patternfly/react-core';
import * as Icons from '@patternfly/react-icons';
import * as Hooks from '@patternfly/react-hooks';`,
			pattern:        "Icons",
			expectedCount:  1,
			expectedLine:   1,
			expectedColumn: 12,
		},
		{
			name: "Namespace with special chars in package name",
			fileContent: `import * as Util from '@company/util-package';
export const test = Util.helper();`,
			pattern:        "Util",
			expectedCount:  1,
			expectedLine:   0,
			expectedColumn: 12,
		},
		{
			name: "Word boundary in namespace import",
			fileContent: `import * as Card from '@patternfly/react-core';
import * as CardHelper from './helpers';`,
			pattern:        "Card",
			expectedCount:  1,
			expectedLine:   0,
			expectedColumn: 12, // Should only match first, not CardHelper
		},
		{
			name: "Stress test - very long multiline import",
			fileContent: `import {
  Button,
  Card,
  CardBody,
  CardHeader,
  CardTitle,
  Chip,
  ChipGroup,
  Label,
  Badge,
  Alert,
  AlertGroup,
  Modal,
  ModalVariant,
  Toolbar,
  ToolbarContent,
  ToolbarItem,
  Select,
  SelectOption,
  SelectVariant
} from '@patternfly/react-core';`,
			pattern:        "ModalVariant",
			expectedCount:  1,
			expectedLine:   13,
			expectedColumn: 2,
		},
		{
			name: "TypeScript type import - default",
			fileContent: `import type React from 'react';
type Props = { children: React.ReactNode };`,
			pattern:        "React",
			expectedCount:  1,
			expectedLine:   0,
			expectedColumn: 12,
		},
		{
			name: "TypeScript type import - namespace",
			fileContent: `import type * as Types from './types';
type MyType = Types.User;`,
			pattern:        "Types",
			expectedCount:  1,
			expectedLine:   0,
			expectedColumn: 17,
		},
		{
			name: "TypeScript type import - multiline",
			fileContent: `import type {
  ButtonProps,
  CardProps
} from '@patternfly/react-core';`,
			pattern:        "CardProps",
			expectedCount:  1,
			expectedLine:   2,
			expectedColumn: 2,
		},
		{
			name: "Pattern as prefix of another symbol on same line",
			fileContent: `import { CardFooter, Card } from '@patternfly/react-core';
export const MyCard = Card;`,
			pattern:        "Card",
			expectedCount:  1,
			expectedLine:   0,
			expectedColumn: 21, // Should find standalone "Card", not "Card" in "CardFooter"
		},
		{
			name: "Pattern in middle of other symbols",
			fileContent: `import { Button, Card, CardBody } from '@patternfly/react-core';`,
			pattern:        "Card",
			expectedCount:  1,
			expectedLine:   0,
			expectedColumn: 17, // Should find standalone "Card", not "Card" in "CardBody"
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create test file
			testFile := filepath.Join(tmpDir, tc.name+".tsx")
			err := os.WriteFile(testFile, []byte(tc.fileContent), 0644)
			if err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}

			// Create NodeServiceClient with test file
			sc := &nodejs.NodeServiceClient{}
			files := []nodejs.FileInfo{
				{
					Path:   "file://" + testFile,
					LangID: "typescriptreact",
				},
			}

			// Call findImportStatements
			locations := sc.FindImportStatementsPublic(tc.pattern, files)

			// Verify count
			if len(locations) != tc.expectedCount {
				t.Errorf("Expected %d locations, got %d", tc.expectedCount, len(locations))
				return
			}

			// If we expect results, verify position
			if tc.expectedCount > 0 {
				if locations[0].Position.Line != tc.expectedLine {
					t.Errorf("Expected line %d, got %d", tc.expectedLine, locations[0].Position.Line)
				}
				if locations[0].Position.Character != tc.expectedColumn {
					t.Errorf("Expected column %d, got %d", tc.expectedColumn, locations[0].Position.Character)
				}
			}
		})
	}
}

// Test isIdentifierChar helper
func TestIsIdentifierChar(t *testing.T) {
	tests := []struct {
		char     rune
		expected bool
	}{
		// Valid identifier characters
		{'a', true},
		{'Z', true},
		{'0', true},
		{'_', true},
		{'$', true},

		// Invalid identifier characters
		{' ', false},
		{'{', false},
		{'}', false},
		{',', false},
		{';', false},
		{'\n', false},
		{'\t', false},
	}

	sc := &nodejs.NodeServiceClient{}

	for _, tt := range tests {
		t.Run(string(tt.char), func(t *testing.T) {
			result := sc.IsIdentifierCharPublic(tt.char)
			if result != tt.expected {
				t.Errorf("isIdentifierChar(%q) = %v, want %v", tt.char, result, tt.expected)
			}
		})
	}
}

// Test word boundary detection in import matching
func TestImportWordBoundaries(t *testing.T) {
	tmpDir := t.TempDir()

	testCases := []struct {
		name          string
		fileContent   string
		pattern       string
		shouldMatch   bool
		description   string
	}{
		{
			name: "Exact match",
			fileContent: `import { Card } from '@patternfly/react-core';`,
			pattern:     "Card",
			shouldMatch: true,
			description: "Should match exact symbol name",
		},
		{
			name: "Avoid substring match - prefix",
			fileContent: `import { CardBody } from '@patternfly/react-core';`,
			pattern:     "Card",
			shouldMatch: false,
			description: "Should NOT match when pattern is prefix of symbol",
		},
		{
			name: "Avoid substring match - suffix",
			fileContent: `import { MyCard } from '@patternfly/react-core';`,
			pattern:     "Card",
			shouldMatch: false,
			description: "Should NOT match when pattern is suffix of symbol",
		},
		{
			name: "Multiple symbols - match first",
			fileContent: `import { Button, Card, Chip } from '@patternfly/react-core';`,
			pattern:     "Button",
			shouldMatch: true,
			description: "Should match first symbol in list",
		},
		{
			name: "Multiple symbols - match middle",
			fileContent: `import { Button, Card, Chip } from '@patternfly/react-core';`,
			pattern:     "Card",
			shouldMatch: true,
			description: "Should match middle symbol in list",
		},
		{
			name: "Multiple symbols - match last",
			fileContent: `import { Button, Card, Chip } from '@patternfly/react-core';`,
			pattern:     "Chip",
			shouldMatch: true,
			description: "Should match last symbol in list",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create test file
			testFile := filepath.Join(tmpDir, tc.name+".tsx")
			err := os.WriteFile(testFile, []byte(tc.fileContent), 0644)
			if err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}

			// Create NodeServiceClient with test file
			sc := &nodejs.NodeServiceClient{}
			files := []nodejs.FileInfo{
				{
					Path:   "file://" + testFile,
					LangID: "typescriptreact",
				},
			}

			// Call findImportStatements
			locations := sc.FindImportStatementsPublic(tc.pattern, files)

			matched := len(locations) > 0
			if matched != tc.shouldMatch {
				t.Errorf("%s: Expected match=%v, got match=%v (found %d locations)",
					tc.description, tc.shouldMatch, matched, len(locations))
			}
		})
	}
}

// Test edge cases for import pattern matching
func TestImportEdgeCases(t *testing.T) {
	tmpDir := t.TempDir()

	testCases := []struct {
		name        string
		fileContent string
		pattern     string
		expectedLen int
	}{
		{
			name: "Empty file",
			fileContent: ``,
			pattern:     "Button",
			expectedLen: 0,
		},
		{
			name: "No imports",
			fileContent: `const Button = () => <div />;
export default Button;`,
			pattern:     "Button",
			expectedLen: 0,
		},
		{
			name: "Import in comment",
			fileContent: `// import { Button } from '@patternfly/react-core';
const x = 1;`,
			pattern:     "Button",
			expectedLen: 1, // Note: Current implementation doesn't filter comments - acceptable limitation
		},
		{
			name: "Import in string",
			fileContent: `const code = "import { Button } from '@patternfly/react-core';";`,
			pattern:     "Button",
			expectedLen: 1, // Note: Current implementation doesn't filter strings - acceptable limitation
		},
		{
			name: "Whitespace variations",
			fileContent: `import  {  Button  }  from  '@patternfly/react-core' ;`,
			pattern:     "Button",
			expectedLen: 1,
		},
		{
			name: "Single quotes",
			fileContent: `import { Button } from '@patternfly/react-core';`,
			pattern:     "Button",
			expectedLen: 1,
		},
		{
			name: "Double quotes",
			fileContent: `import { Button } from "@patternfly/react-core";`,
			pattern:     "Button",
			expectedLen: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create test file
			testFile := filepath.Join(tmpDir, tc.name+".tsx")
			err := os.WriteFile(testFile, []byte(tc.fileContent), 0644)
			if err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}

			// Create NodeServiceClient with test file
			sc := &nodejs.NodeServiceClient{}
			files := []nodejs.FileInfo{
				{
					Path:   "file://" + testFile,
					LangID: "typescriptreact",
				},
			}

			// Call findImportStatements
			locations := sc.FindImportStatementsPublic(tc.pattern, files)

			if len(locations) != tc.expectedLen {
				t.Errorf("Expected %d locations, got %d", tc.expectedLen, len(locations))
			}
		})
	}
}

// Benchmark findImportStatements performance
func BenchmarkFindImportStatements(b *testing.B) {
	tmpDir := b.TempDir()

	// Create a realistic test file
	testContent := `import React from 'react';
import {
  Button,
  Card,
  CardBody,
  CardHeader,
  Chip,
  ChipGroup,
  Label,
  LabelGroup
} from '@patternfly/react-core';
import { useState, useEffect } from 'react';

export const MyComponent = () => {
  const [open, setOpen] = useState(false);

  return (
    <Card>
      <CardHeader>
        <Button onClick={() => setOpen(true)}>Click me</Button>
      </CardHeader>
      <CardBody>
        <ChipGroup>
          <Chip>Tag 1</Chip>
          <Chip>Tag 2</Chip>
        </ChipGroup>
      </CardBody>
    </Card>
  );
};`

	testFile := filepath.Join(tmpDir, "test.tsx")
	err := os.WriteFile(testFile, []byte(testContent), 0644)
	if err != nil {
		b.Fatalf("Failed to create test file: %v", err)
	}

	sc := &nodejs.NodeServiceClient{}
	files := []nodejs.FileInfo{
		{
			Path:   "file://" + testFile,
			LangID: "typescriptreact",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sc.FindImportStatementsPublic("Button", files)
	}
}
