package java

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/konveyor/analyzer-lsp/engine"
	"go.lsp.dev/uri"
)

const (
	FILE_URI_PREFIX = "konveyor-jdt"
)

var _ engine.CodeSnip = &javaProvider{}

func (p *javaProvider) GetCodeSnip(u uri.URI, loc engine.Location) (string, error) {
	ur := string(u)
	if !strings.Contains(ur, FILE_URI_PREFIX) {
		return "", fmt.Errorf("invalid uri, must be for %s", FILE_URI_PREFIX)
	}
	ur = strings.TrimPrefix(ur, fmt.Sprintf("%s://contents", FILE_URI_PREFIX))

	parts := strings.Split(ur, "?")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid uri, can not find correct query string")
	}
	path := parts[0]
	queryString := parts[1]

	queryStringParts := strings.Split(queryString, "&")
	if len(queryStringParts) != 2 {
		return "", fmt.Errorf("invalid uri, can not find correct parts for query string")
	}

	packageName := strings.Split(queryStringParts[0], "=")[1]
	sourceRange := strings.Split(queryStringParts[1], "=")[1]
	isSourceRange, err := strconv.ParseBool(sourceRange)
	if err != nil {
		return "", fmt.Errorf("invalid boolean set for source range")
	}

	if isSourceRange {
		// If there is a source range, we know we know there is a sources jar
		jarName := filepath.Base(path)
		s := strings.TrimSuffix(jarName, ".jar")
		s = fmt.Sprintf("%v-sources.jar", s)
		jarPath := filepath.Join(filepath.Dir(path), s)

		path := filepath.Join(strings.Split(strings.TrimSuffix(packageName, ".class"), ".")...)

		javaFileName := fmt.Sprintf("%s.java", filepath.Base(path))
		if i := strings.Index(javaFileName, "$"); i > 0 {
			javaFileName = fmt.Sprintf("%v.java", javaFileName[0:i])
		}

		javaFileAbsolutePath := filepath.Join(filepath.Dir(jarPath), filepath.Dir(path), javaFileName)

		if _, err := os.Stat(javaFileAbsolutePath); err != nil {
			cmd := exec.Command("jar", "xf", filepath.Base(jarPath))
			cmd.Dir = filepath.Dir(jarPath)
			err := cmd.Run()
			if err != nil {
				fmt.Printf("\n java%v", err)
				return "", err
			}
		}

		snip, err := p.scanFile(javaFileAbsolutePath, loc)
		if err != nil {
			fmt.Printf("\n%v", err)
			return "", err
		}
		return snip, nil
	}

	return "", nil
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
