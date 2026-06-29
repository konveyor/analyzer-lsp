package nodejs_external_provider

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/konveyor/analyzer-lsp/engine"
	"go.lsp.dev/uri"
)

var _ engine.CodeSnip = &nodejsProvider{}

func (p *nodejsProvider) GetCodeSnip(u uri.URI, loc engine.Location) (string, error) {
	if !strings.HasPrefix(string(u), uri.FileScheme+"://") {
		return "", fmt.Errorf("invalid file uri, must use %s scheme", uri.FileScheme)
	}
	return p.scanFile(u.Filename(), loc)
}

func (p *nodejsProvider) scanFile(path string, loc engine.Location) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	lineNumber := 0
	codeSnip := ""
	paddingSize := len(strconv.Itoa(loc.EndPosition.Line + p.contextLines))
	for scanner.Scan() {
		if (lineNumber-p.contextLines) == loc.EndPosition.Line {
			codeSnip = codeSnip + fmt.Sprintf("%*d  %v", paddingSize, lineNumber+1, scanner.Text())
			break
		}
		if (lineNumber+p.contextLines) >= loc.StartPosition.Line {
			codeSnip = codeSnip + fmt.Sprintf("%*d  %v\n", paddingSize, lineNumber+1, scanner.Text())
		}
		lineNumber++
	}
	return codeSnip, nil
}
