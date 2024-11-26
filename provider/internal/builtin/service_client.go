package builtin

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/antchfx/jsonquery"
	"github.com/antchfx/xmlquery"
	"github.com/antchfx/xpath"
	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/konveyor/analyzer-lsp/tracing"
	"go.lsp.dev/uri"
	"gopkg.in/yaml.v2"
)

type builtinServiceClient struct {
	config provider.InitConfig
	tags   map[string]bool
	provider.UnimplementedDependenciesComponent
	log logr.Logger

	cacheMutex    sync.RWMutex
	locationCache map[string]float64
	includedPaths []string
}

type fileTemplateContext struct {
	Filepaths []string `json:"filepaths,omitempty"`
}

var _ provider.ServiceClient = &builtinServiceClient{}

func (p *builtinServiceClient) Stop() {}

func (p *builtinServiceClient) Evaluate(ctx context.Context, cap string, conditionInfo []byte) (provider.ProviderEvaluateResponse, error) {
	var cond builtinCondition
	err := yaml.Unmarshal(conditionInfo, &cond)
	if err != nil {
		return provider.ProviderEvaluateResponse{}, fmt.Errorf("unable to get query info: %v", err)
	}
	log := p.log.WithValues("ruleID", cond.ProviderContext.RuleID)
	log.V(5).Info("builtin condition context", "condition", cond, "provider context", cond.ProviderContext)
	response := provider.ProviderEvaluateResponse{Matched: false}
	switch cap {
	case "file":
		c := cond.File
		if c.Pattern == "" {
			return response, fmt.Errorf("could not parse provided file pattern as string: %v", conditionInfo)
		}
		matchingFiles := []string{}
		if ok, paths := cond.ProviderContext.GetScopedFilepaths(); ok {
			regex, _ := regexp.Compile(c.Pattern)
			for _, path := range paths {
				matched := false
				if regex != nil {
					matched = regex.MatchString(path)
				} else {
					// TODO(fabianvf): is a fileglob style pattern sufficient or do we need regexes?
					matched, err = filepath.Match(c.Pattern, path)
					if err != nil {
						continue
					}
				}
				if matched {
					matchingFiles = append(matchingFiles, path)
				}
			}
		} else {
			matchingFiles, err = findFilesMatchingPattern(p.config.Location, c.Pattern)
			if err != nil {
				return response, fmt.Errorf("unable to find files using pattern `%s`: %v", c.Pattern, err)
			}
		}

		response.TemplateContext = map[string]interface{}{"filepaths": matchingFiles}
		for _, match := range matchingFiles {
			absPath := match
			if !filepath.IsAbs(match) {
				absPath, err = filepath.Abs(match)
				if err != nil {
					p.log.V(5).Error(err, "failed to get absolute path to file", "path", match)
					absPath = match
				}
			}
			if !p.isFileIncluded(absPath) {
				continue
			}
			response.Incidents = append(response.Incidents, provider.IncidentContext{
				FileURI: uri.File(absPath),
			})
		}
		response.Matched = len(response.Incidents) > 0
		return response, nil
	case "filecontent":
		c := cond.Filecontent
		if c.Pattern == "" {
			return response, fmt.Errorf("could not parse provided regex pattern as string: %v", conditionInfo)
		}

		var outputBytes []byte
		//Runs on Windows using PowerShell.exe and Unix based systems using grep
		outputBytes, err := runOSSpecificGrepCommand(c.Pattern, p.config.Location, cond.ProviderContext)
		if err != nil {
			return response, err
		}
		matches := []string{}
		outputString := strings.TrimSpace(string(outputBytes))
		if outputString != "" {
			matches = append(matches, strings.Split(outputString, "\n")...)
		}

		for _, match := range matches {
			var pieces []string
			pieces, err := parseGrepOutputForFileContent(match)
			if err != nil {
				return response, fmt.Errorf("could not parse grep output '%s' for the Pattern '%v': %v ", match, c.Pattern, err)
			}

			containsFile, err := provider.FilterFilePattern(c.FilePattern, pieces[0])
			if err != nil {
				return response, err
			}
			if !containsFile {
				continue
			}

			absPath, err := filepath.Abs(pieces[0])
			if err != nil {
				absPath = pieces[0]
			}

			if !p.isFileIncluded(absPath) {
				continue
			}

			lineNumber, err := strconv.Atoi(pieces[1])
			if err != nil {
				return response, fmt.Errorf("cannot convert line number string to integer")
			}

			response.Incidents = append(response.Incidents, provider.IncidentContext{
				FileURI:    uri.File(absPath),
				LineNumber: &lineNumber,
				Variables: map[string]interface{}{
					"matchingText": pieces[2],
				},
				CodeLocation: &provider.Location{
					StartPosition: provider.Position{Line: float64(lineNumber)},
					EndPosition:   provider.Position{Line: float64(lineNumber)},
				},
			})
		}
		if len(response.Incidents) != 0 {
			response.Matched = true
		}
		return response, nil
	case "xml":
		query, err := xpath.CompileWithNS(cond.XML.XPath, cond.XML.Namespaces)
		if query == nil || err != nil {
			return response, fmt.Errorf("could not parse provided xpath query '%s': %v", cond.XML.XPath, err)
		}
		filePaths := []string{}
		if ok, paths := cond.ProviderContext.GetScopedFilepaths(); ok {
			if len(cond.XML.Filepaths) > 0 {
				newPaths := []string{}
				// Sometimes rules have hardcoded filepaths
				// Or use other searching to get them. If so, then we
				// Should respect that added filter on the scoped filepaths
				for _, p := range cond.XML.Filepaths {
					for _, path := range paths {
						if p == path {
							newPaths = append(newPaths, path)
						}
						if filepath.Base(path) == p {
							newPaths = append(newPaths, path)
						}
					}
				}
				if len(newPaths) == 0 {
					// There are no files to search, return.
					return response, nil
				}
				filePaths = newPaths
			} else {
				filePaths = paths
			}
		} else if len(cond.XML.Filepaths) > 0 {
			filePaths = cond.XML.Filepaths
		}
		xmlFiles, err := findXMLFiles(p.config.Location, filePaths, log)
		if err != nil {
			return response, fmt.Errorf("unable to find XML files: %v", err)
		}
		for _, file := range xmlFiles {
			nodes, err := queryXMLFile(file, query)
			if err != nil {
				log.V(5).Error(err, "failed to query xml file", "file", file)
				continue
			}
			if len(nodes) != 0 {
				response.Matched = true
				for _, node := range nodes {
					absPath, err := filepath.Abs(file)
					if err != nil {
						absPath = file
					}
					if !p.isFileIncluded(absPath) {
						continue
					}
					incident := provider.IncidentContext{
						FileURI: uri.File(absPath),
						Variables: map[string]interface{}{
							"matchingXML": node.OutputXML(false),
							"innerText":   node.InnerText(),
							"data":        node.Data,
						},
					}
					content := strings.TrimSpace(node.InnerText())
					if content == "" {
						content = node.Data
					}
					location, err := p.getLocation(ctx, absPath, content)
					if err == nil {
						incident.CodeLocation = &location
						lineNo := int(location.StartPosition.Line)
						incident.LineNumber = &lineNo
					}
					response.Incidents = append(response.Incidents, incident)
				}
			}
		}

		return response, nil
	case "xmlPublicID":
		regex, err := regexp.Compile(cond.XMLPublicID.Regex)
		if err != nil {
			return response, fmt.Errorf("could not parse provided public-id regex '%s': %v", cond.XMLPublicID.Regex, err)
		}
		query, err := xpath.CompileWithNS("//*[@public-id]", cond.XMLPublicID.Namespaces)
		if query == nil || err != nil {
			return response, fmt.Errorf("could not parse public-id xml query '%s': %v", cond.XML.XPath, err)
		}
		filePaths := []string{}
		if ok, paths := cond.ProviderContext.GetScopedFilepaths(); ok {
			if len(cond.XML.Filepaths) > 0 {
				newPaths := []string{}
				// Sometimes rules have hardcoded filepaths
				// Or use other searching to get them. If so, then we
				// Should respect that added filter on the scoped filepaths
				for _, p := range cond.XML.Filepaths {
					for _, path := range paths {
						if p == path {
							newPaths = append(newPaths, path)
						}
					}
				}
				if len(newPaths) == 0 {
					// There are no files to search, return.
					return response, nil
				}
				filePaths = newPaths
			} else {
				filePaths = paths
			}
		} else if len(cond.XML.Filepaths) > 0 {
			filePaths = cond.XML.Filepaths
		}
		xmlFiles, err := findXMLFiles(p.config.Location, filePaths, p.log)
		if err != nil {
			return response, fmt.Errorf("unable to find XML files: %v", err)
		}
		for _, file := range xmlFiles {
			nodes, err := queryXMLFile(file, query)
			if err != nil {
				log.Error(err, "failed to query xml file", "file", file)
				continue
			}

			for _, node := range nodes {
				// public-id attribute regex match check
				for _, attr := range node.Attr {
					if attr.Name.Local == "public-id" {
						if regex.MatchString(attr.Value) {
							response.Matched = true
							absPath, err := filepath.Abs(file)
							if err != nil {
								absPath = file
							}
							if !p.isFileIncluded(absPath) {
								continue
							}
							response.Incidents = append(response.Incidents, provider.IncidentContext{
								FileURI: uri.File(absPath),
								Variables: map[string]interface{}{
									"matchingXML": node.OutputXML(false),
									"innerText":   node.InnerText(),
									"data":        node.Data,
								},
							})
						}
						break
					}
				}
			}
		}

		return response, nil
	case "json":
		query := cond.JSON.XPath
		if query == "" {
			return response, fmt.Errorf("could not parse provided xpath query as string: %v", conditionInfo)
		}
		pattern := "*.json"
		filePaths := []string{}
		if ok, paths := cond.ProviderContext.GetScopedFilepaths(); ok {
			filePaths = paths
		} else if len(cond.XML.Filepaths) > 0 {
			filePaths = cond.JSON.Filepaths
		}
		jsonFiles, err := provider.GetFiles(p.config.Location, filePaths, pattern)
		if err != nil {
			return response, fmt.Errorf("unable to find files using pattern `%s`: %v", pattern, err)
		}
		for _, file := range jsonFiles {
			f, err := os.Open(file)
			if err != nil {
				log.V(5).Error(err, "error opening json file", "file", file)
				continue
			}
			doc, err := jsonquery.Parse(f)
			if err != nil {
				log.V(5).Error(err, "error parsing json file", "file", file)
				continue
			}
			list, err := jsonquery.QueryAll(doc, query)
			if err != nil {
				return response, err
			}
			if len(list) != 0 {
				response.Matched = true
				for _, node := range list {
					absPath, err := filepath.Abs(file)
					if err != nil {
						absPath = file
					}
					if !p.isFileIncluded(absPath) {
						continue
					}
					incident := provider.IncidentContext{
						FileURI: uri.File(absPath),
						Variables: map[string]interface{}{
							"matchingJSON": node.InnerText(),
							"data":         node.Data,
						},
					}
					location, err := p.getLocation(ctx, absPath, node.InnerText())
					if err == nil {
						incident.CodeLocation = &location
						lineNo := int(location.StartPosition.Line)
						incident.LineNumber = &lineNo
					}
					response.Incidents = append(response.Incidents, incident)
				}
			}
		}
		return response, nil
	case "hasTags":
		found := true
		for _, tag := range cond.HasTags {
			if _, exists := cond.ProviderContext.Tags[tag]; !exists {
				if _, exists := p.tags[tag]; !exists {
					found = false
					break
				}
			}
		}
		if found {
			response.Matched = true
			response.Incidents = append(response.Incidents, provider.IncidentContext{
				Variables: map[string]interface{}{
					"tags": cond.HasTags,
				},
			})
		}
		return response, nil
	default:
		return response, fmt.Errorf("capability must be one of %v, not %s", capabilities, cap)
	}
}

// getLocation attempts to get code location for given content in JSON / XML files
func (b *builtinServiceClient) getLocation(ctx context.Context, path, content string) (provider.Location, error) {
	ctx, span := tracing.StartNewSpan(ctx, "getLocation")
	defer span.End()
	location := provider.Location{}

	parts := strings.Split(content, "\n")
	if len(parts) < 1 {
		return location, fmt.Errorf("unable to get code location, empty content")
	} else if len(parts) > 5 {
		// limit content to search
		parts = parts[:5]
	}
	lines := []string{}
	for _, part := range parts {
		line := strings.Trim(part, " ")
		line = strings.ReplaceAll(line, "\t", "")
		line = regexp.QuoteMeta(line)
		lines = append(lines, line)
	}
	// remove leading and trailing empty lines
	if len(lines) > 0 && lines[0] == "" {
		lines = lines[1:]
	}
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) < 1 || strings.Join(lines, "") == "" {
		return location, fmt.Errorf("unable to get code location, empty content")
	}
	pattern := fmt.Sprintf(".*?%s", strings.Join(lines, ".*?"))
	cacheKey := fmt.Sprintf("%s-%s", path, pattern)
	b.cacheMutex.RLock()
	val, exists := b.locationCache[cacheKey]
	b.cacheMutex.RUnlock()
	if exists {
		if val == -1 {
			return location, fmt.Errorf("unable to get location due to a previous error")
		}
		return provider.Location{
			StartPosition: provider.Position{
				Line: float64(val),
			},
			EndPosition: provider.Position{
				Line: float64(val),
			},
		}, nil
	}

	defer func() {
		b.cacheMutex.Lock()
		b.locationCache[cacheKey] = location.StartPosition.Line
		b.cacheMutex.Unlock()
	}()

	location.StartPosition.Line = -1
	lineNumber, err := provider.MultilineGrep(ctx, len(lines), path, pattern)
	if err != nil || lineNumber == -1 {
		return location, fmt.Errorf("unable to get location in file %s - %w", path, err)
	}
	location.StartPosition.Line = float64(lineNumber)
	location.EndPosition.Line = float64(lineNumber)
	return location, nil
}

func findFilesMatchingPattern(root, pattern string) ([]string, error) {
	var regex *regexp.Regexp
	// if the regex doesn't compile, we'll default to using filepath.Match on the pattern directly
	regex, _ = regexp.Compile(pattern)
	matches := []string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		var matched bool
		if regex != nil {
			matched = regex.MatchString(d.Name())
		} else {
			// TODO(fabianvf): is a fileglob style pattern sufficient or do we need regexes?
			matched, err = filepath.Match(pattern, d.Name())
			if err != nil {
				return err
			}
		}
		if matched {
			matches = append(matches, path)
		}
		return nil
	})
	return matches, err
}

func findXMLFiles(baseLocation string, filePaths []string, log logr.Logger) ([]string, error) {
	patterns := []string{"*.xml", "*.xhtml"}
	// TODO(fabianvf): how should we scope the files searched here?
	xmlFiles, err := provider.GetFiles(baseLocation, filePaths, patterns...)
	return xmlFiles, err
}

func queryXMLFile(filePath string, query *xpath.Expr) (nodes []*xmlquery.Node, err error) {
	f, err := os.Open(filePath)
	if err != nil {
		fmt.Printf("unable to open file '%s': %v\n", filePath, err)
		return nil, err
	}
	defer f.Close()
	// TODO This should start working if/when this merges and releases: https://github.com/golang/go/pull/56848
	var doc *xmlquery.Node
	doc, err = xmlquery.ParseWithOptions(f, xmlquery.ParserOptions{Decoder: &xmlquery.DecoderOptions{Strict: false}})
	if err != nil {
		if err.Error() == "xml: unsupported version \"1.1\"; only version 1.0 is supported" {
			// TODO HACK just pretend 1.1 xml documents are 1.0 for now while we wait for golang to support 1.1
			var b []byte
			b, err = os.ReadFile(filePath)
			if err != nil {
				fmt.Printf("unable to parse xml file '%s': %v\n", filePath, err)
				return nil, err
			}
			docString := strings.Replace(string(b), "<?xml version=\"1.1\"", "<?xml version = \"1.0\"", 1)
			doc, err = xmlquery.Parse(strings.NewReader(docString))
			if err != nil {
				fmt.Printf("unable to parse xml file '%s': %v\n", filePath, err)
				return nil, err
			}
		} else {
			fmt.Printf("unable to parse xml file '%s': %v\n", filePath, err)
			return nil, err
		}
	}
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("recovered panic from xpath query search with err - %v", r)
		}
	}()
	nodes = xmlquery.QuerySelectorAll(doc, query)
	return nodes, err
}

// filterByIncludedPaths given a list of file paths,
// filters-out the ones not present in includedPaths
func (b *builtinServiceClient) isFileIncluded(absolutePath string) bool {
	if b.includedPaths == nil || len(b.includedPaths) == 0 {
		return true
	}

	getSegments := func(path string) []string {
		segments := []string{}
		path = filepath.Clean(path)
		for _, segment := range strings.Split(
			path, string(os.PathSeparator)) {
			if segment != "" {
				segments = append(segments, segment)
			}
		}
		return segments
	}

	for _, path := range b.includedPaths {
		includedPath := filepath.Join(b.config.Location, path)
		if absPath, err := filepath.Abs(includedPath); err == nil {
			includedPath = absPath
		}
		pathSegments := getSegments(absolutePath)
		if stat, err := os.Stat(includedPath); err == nil && stat.IsDir() {
			pathSegments = getSegments(filepath.Dir(absolutePath))
		}
		includedPathSegments := getSegments(includedPath)
		if len(pathSegments) >= len(includedPathSegments) &&
			strings.HasPrefix(strings.Join(pathSegments, ""),
				strings.Join(includedPathSegments, "")) {
			return true
		}
	}
	b.log.V(7).Info("excluding file from search", "file", absolutePath)
	return false
}

func parseGrepOutputForFileContent(match string) ([]string, error) {
	// This will parse the output of the PowerShell/grep in the form
	// "Filepath:Linenumber:Matchingtext" to return string array of path, line number and matching text
	// works with handling both windows and unix based file paths eg: "C:\path\to\file" and "/path/to/file"
	re, err := regexp.Compile(`^(.*?):(\d+):(.*)$`)
	if err != nil {
		return nil, fmt.Errorf("failed to compile regular expression: %v", err)
	}
	submatches := re.FindStringSubmatch(match)
	if len(submatches) != 4 {
		return nil, fmt.Errorf(
			"malformed response from file search, cannot parse result '%s' with pattern %#q", match, re)
	}
	return submatches[1:], nil
}

func runOSSpecificGrepCommand(pattern string, location string, providerContext provider.ProviderContext) ([]byte, error) {
	var outputBytes []byte
	var err error
	var utilName string

	if runtime.GOOS == "windows" {
		utilName = "powershell.exe"
		// Windows does not have grep, so we use PowerShell.exe's Select-String instead
		// This is a workaround until we can find a better solution
		psScript := `
		$pattern = $env:PATTERN
		$location = $env:FILEPATH
		Get-ChildItem -Path $location -Recurse -File | ForEach-Object {
			$file = $_    
			# Search for the pattern in the file
			Select-String -Path $file.FullName -Pattern $pattern -AllMatches | ForEach-Object { 
				foreach ($match in $_.Matches) { 
					"{0}:{1}:{2}" -f $file.FullName, $_.LineNumber, $match.Value
				} 
			}
		}`
		findstr := exec.Command(utilName, "-Command", psScript)
		findstr.Env = append(os.Environ(), "PATTERN="+pattern, "FILEPATH="+location)
		outputBytes, err = findstr.Output()

	} else if runtime.GOOS == "darwin" {
		cmd := fmt.Sprintf(
			`find %v -type f | \
		while read file; do perl -ne '/(%v)/ && print "$ARGV:$.:$1\n";' "$file"; done`,
			location, pattern,
		)
		findstr := exec.Command("/bin/sh", "-c", cmd)
		outputBytes, err = findstr.Output()
	} else {
		grep := exec.Command("grep", "-o", "-n", "-R", "-P", pattern)
		if ok, paths := providerContext.GetScopedFilepaths(); ok {
			grep.Args = append(grep.Args, paths...)
		} else {
			grep.Args = append(grep.Args, location)
		}
		outputBytes, err = grep.Output()
	}
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok && exitError.ExitCode() == 1 {
			return nil, nil
		}
		return nil, fmt.Errorf("could not run '%s' with provided pattern %+v", utilName, err)
	}

	return outputBytes, nil
}
