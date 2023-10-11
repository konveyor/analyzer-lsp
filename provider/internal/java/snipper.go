package java

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/konveyor/analyzer-lsp/engine"
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
	readFile, err := os.Open(path)
	if err != nil {
		p.Log.V(5).Error(err, "Unable to read file")
		return "", err
	}
	defer readFile.Close()

	scanner := bufio.NewScanner(readFile)
	lineNumber := 0
	codeSnip := ""
	paddingSize := len(strconv.Itoa(loc.EndPosition.Line + p.config.ContextLines))
	for scanner.Scan() {
		if (lineNumber - p.config.ContextLines) == loc.EndPosition.Line {
			codeSnip = codeSnip + fmt.Sprintf("%*d  %v", paddingSize, lineNumber+1, scanner.Text())
			break
		}
		if (lineNumber + p.config.ContextLines) >= loc.StartPosition.Line {
			codeSnip = codeSnip + fmt.Sprintf("%*d  %v\n", paddingSize, lineNumber+1, scanner.Text())
		}
		lineNumber += 1
	}
	return codeSnip, nil
}
