package java

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/konveyor/analyzer-lsp/engine"
	"github.com/konveyor/analyzer-lsp/provider"
	"go.lsp.dev/uri"
)

const (
	JDT_CLASS_FILE_URI_PREFIX = "konveyor-jdt"
)

var _ engine.CodeSnip = &javaProvider{}

func (p *javaProvider) GetCodeSnip(u uri.URI, loc engine.Location) (string, error) {
	if !strings.Contains(string(u), uri.FileScheme) {
		return "", fmt.Errorf("invalid file uri, must be for %s", JDT_CLASS_FILE_URI_PREFIX)
	}
	snip, err := p.scanFile(u.Filename(), loc)
	if err != nil {
		return "", err
	}
	return snip, nil
}

func (p *javaProvider) scanFile(path string, loc engine.Location) (string, error) {
	var content []byte
	var err error
	if p.encoding != "" {
		content, err = provider.OpenFileWithEncoding(path, p.encoding)
		if err != nil {
			p.Log.Error(err, "failed to convert file encoding, using original content", "file", path)
			content, err = os.ReadFile(path)
			if err != nil {
				return "", err
			}
		}
	} else {
		content, err = os.ReadFile(path)
		if err != nil {
			return "", err
		}
	}

	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	lineNumber := 0
	codeSnip := ""
	paddingSize := len(strconv.Itoa(loc.EndPosition.Line + p.contextLines))
	for scanner.Scan() {
		if (lineNumber - p.contextLines) == loc.EndPosition.Line {
			codeSnip = codeSnip + fmt.Sprintf("%*d  %v", paddingSize, lineNumber+1, scanner.Text())
			break
		}
		if (lineNumber + p.contextLines) >= loc.StartPosition.Line {
			codeSnip = codeSnip + fmt.Sprintf("%*d  %v\n", paddingSize, lineNumber+1, scanner.Text())
		}
		lineNumber += 1
	}
	return codeSnip, nil
}
