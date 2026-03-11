package builtin

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/antchfx/jsonquery"
	"github.com/antchfx/xmlquery"
	"github.com/antchfx/xpath"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/konveyor/analyzer-lsp/tracing"
)

// maxCacheFileSize is the maximum file size (in bytes) that will be read
// during Prepare()'s incident caching. Files larger than this are skipped.
const maxCacheFileSize = 100 * 1024 * 1024 // 100MB

// numWorkers is the number of goroutines used for both the initial
// incident cache population and the cache refresh worker pool.
// TODO: make numWorkers configurable via providerSpecificConfig
const numWorkers = 5

// cacheRefreshRequest is sent from the working copy manager to the
// builtinServiceClient after a file change has been written to disk.
type cacheRefreshRequest struct {
	// originalPath is the real file path (used as cache key)
	originalPath string
	// contentPath is where the current content lives:
	// - temp file for unsaved changes
	// - original path for saved changes (re-read from disk)
	contentPath string
}

// incidentCacheKey uniquely identifies a (capability, search params) pair.
type incidentCacheKey struct {
	cap    string
	params string
}

// parsedCondition holds a pre-parsed, pre-compiled condition ready for execution
// during Prepare()'s per-file processing. Compiling regexes and XPath expressions
// once here avoids re-compilation for every file.
type parsedCondition struct {
	cacheKey            incidentCacheKey
	cap                 string
	condition           builtinCondition
	compiledXPath       *xpath.Expr    // xml, xmlPublicID
	compiledRegex       *regexp.Regexp // xmlPublicID
	compiledFilePattern *regexp.Regexp // filecontent FilePattern, nil if empty or invalid regex
	jsonXPath           string         // json
}

// makeIncidentCacheKey builds a canonical cache key from a condition's
// parsed fields (Pattern, XPath, etc.).
func makeIncidentCacheKey(cap string, cond builtinCondition) incidentCacheKey {
	switch cap {
	case "filecontent":
		return incidentCacheKey{
			cap:    cap,
			params: fmt.Sprintf("%v-%v", cond.Filecontent.Pattern, cond.Filecontent.FilePattern),
		}
	case "xml":
		return incidentCacheKey{
			cap:    cap,
			params: fmt.Sprintf("%v-%v", cond.XML.XPath, sortedNamespaces(cond.XML.Namespaces)),
		}
	case "xmlPublicID":
		return incidentCacheKey{
			cap:    cap,
			params: fmt.Sprintf("%v-%v", cond.XMLPublicID.Regex, sortedNamespaces(cond.XMLPublicID.Namespaces)),
		}
	case "json":
		return incidentCacheKey{
			cap:    cap,
			params: cond.JSON.XPath,
		}
	default:
		return incidentCacheKey{cap: cap}
	}
}

func sortedNamespaces(ns map[string]string) string {
	if len(ns) == 0 {
		return ""
	}
	keys := make([]string, 0, len(ns))
	for k := range ns {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('|')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(ns[k])
	}
	return b.String()
}

// isChainedCondition returns true if the condition has non-empty Filepaths,
// which are template-expanded at Evaluate() time and cannot be pre-cached.
func isChainedCondition(cap string, cond builtinCondition) bool {
	switch cap {
	case "filecontent":
		return len(cond.Filecontent.Filepaths) > 0
	case "xml":
		return len(cond.XML.Filepaths) > 0
	case "xmlPublicID":
		return len(cond.XMLPublicID.Filepaths) > 0
	case "json":
		return len(cond.JSON.Filepaths) > 0
	default:
		return false
	}
}

// tryIncidentCache attempts to look up cached incidents for a capability.
// If any scoped file has a pending cache refresh, it waits for the refresh
// to complete before returning results. Returns the incidents, whether a
// cache hit occurred, and any error.
func (b *builtinServiceClient) tryIncidentCache(ctx context.Context, cap string, cond builtinCondition, patterns []string) ([]provider.IncidentContext, bool, error) {
	if !b.prepared || len(b.allParsedConditions) == 0 || isChainedCondition(cap, cond) {
		return nil, false, nil
	}
	key := makeIncidentCacheKey(cap, cond)
	scopedFiles, err := b.filterCachedFiles(cond, provider.SearchCriteria{
		Patterns: patterns,
	})
	if err != nil {
		return nil, false, err
	}
	// Wait for any pending cache refreshes to complete before reading
	// from the cache, so Evaluate never returns stale/incomplete results.
	for _, f := range scopedFiles {
		val, pending := b.pendingCacheRefresh.Load(f)
		if !pending {
			continue
		}
		ch := val.(chan struct{})
		select {
		case <-ch:
		case <-ctx.Done():
			return nil, false, nil
		}
	}
	incidents, ok := b.incidentsFromCache(key, scopedFiles)
	return incidents, ok, nil
}

// filterCachedFiles applies per-rule scope constraints and search criteria
// against the cached file index, returning matching files.
func (b *builtinServiceClient) filterCachedFiles(cond builtinCondition, criteria provider.SearchCriteria) ([]string, error) {
	includedPaths, excludedPaths := cond.ProviderContext.GetScopedFilepaths()

	searcher := provider.FileSearcher{
		BasePath: b.config.Location,
		ProviderConfigConstraints: provider.IncludeExcludeConstraints{
			IncludePathsOrPatterns: b.includedPaths,
			ExcludePathsOrPatterns: b.excludedDirs,
		},
		RuleScopeConstraints: provider.IncludeExcludeConstraints{
			IncludePathsOrPatterns: includedPaths,
			ExcludePathsOrPatterns: excludedPaths,
		},
		FailFast: true,
		Log:      b.log,
	}

	return searcher.SearchFromIndex(b.fileIndex, criteria)
}

// mergeIntoCache thread-safely merges incidents for a given cache key and file
// into the incident cache.
func (b *builtinServiceClient) mergeIntoCache(key incidentCacheKey, filePath string, incidents []provider.IncidentContext) {
	if len(incidents) == 0 {
		return
	}
	b.incidentCacheMutex.Lock()
	defer b.incidentCacheMutex.Unlock()
	if b.incidentCache[key] == nil {
		b.incidentCache[key] = make(map[string][]provider.IncidentContext)
	}
	b.incidentCache[key][filePath] = incidents
}

// incidentsFromCache looks up cached incidents for a cache key, filtered to
// only include files in the provided scope set. Returns nil and false on cache miss.
func (b *builtinServiceClient) incidentsFromCache(key incidentCacheKey, scopedFiles []string) ([]provider.IncidentContext, bool) {
	b.incidentCacheMutex.RLock()
	defer b.incidentCacheMutex.RUnlock()
	fileMap, exists := b.incidentCache[key]
	if !exists {
		return nil, false
	}

	var result []provider.IncidentContext
	for _, f := range scopedFiles {
		if incidents, ok := fileMap[f]; ok {
			result = append(result, incidents...)
		}
	}
	return result, true
}

// cacheRefreshWorker consumes from cacheRefreshChan and re-runs all parsed
// conditions against the updated file content, storing results in the cache.
// Each refresh gets a per-file child context so that it can be cancelled
// when a newer change arrives for the same file or on shutdown.
func (b *builtinServiceClient) cacheRefreshWorker() {
	for {
		select {
		case req, ok := <-b.cacheRefreshChan:
			if !ok {
				return
			}
			if !b.prepared {
				continue
			}
			fileCtx, fileCancel := context.WithCancel(b.cacheRefreshCtx)
			b.activeRefreshes.Store(req.originalPath, fileCancel)
			b.refreshCacheForFile(fileCtx, req.originalPath, req.contentPath)
			b.activeRefreshes.Delete(req.originalPath)
			fileCancel()
		case <-b.cacheRefreshCtx.Done():
			return
		}
	}
}

// completePendingRefresh removes the pending entry for a file and closes
// its done channel, unblocking any Evaluate calls waiting in tryIncidentCache.
func (b *builtinServiceClient) completePendingRefresh(filePath string) {
	val, loaded := b.pendingCacheRefresh.LoadAndDelete(filePath)
	if loaded {
		close(val.(chan struct{}))
	}
}

// invalidateCacheForFile removes all cached incidents for the given file path
// across all cache keys.
func (b *builtinServiceClient) invalidateCacheForFile(filePath string) {
	b.incidentCacheMutex.Lock()
	defer b.incidentCacheMutex.Unlock()
	for key, fileMap := range b.incidentCache {
		delete(fileMap, filePath)
		b.incidentCache[key] = fileMap
	}
}

// refreshCacheForFile re-runs all parsed conditions against the given file
// and updates the incident cache. The filePath should be the original file path
// (not the working copy temp path) so cache keys align with Evaluate lookups.
func (b *builtinServiceClient) refreshCacheForFile(ctx context.Context, originalPath string, contentPath string) {
	defer b.completePendingRefresh(originalPath)

	if !b.prepared || len(b.allParsedConditions) == 0 {
		return
	}

	ctx, span := tracing.StartNewSpan(ctx, "refreshCacheForFile")
	defer span.End()

	fileInfo, err := os.Stat(contentPath)
	if err != nil {
		b.log.V(5).Error(err, "failed to stat file for cache refresh", "file", contentPath)
		return
	}
	if fileInfo.Size() > maxCacheFileSize {
		b.log.V(5).Info("skipping large file for cache refresh", "file", contentPath, "size", fileInfo.Size())
		return
	}

	relPath, err := filepath.Rel(b.config.Location, originalPath)
	if err != nil {
		relPath = filepath.Base(originalPath)
	}
	baseName := filepath.Base(originalPath)

	ext := strings.ToLower(filepath.Ext(originalPath))
	isXML := ext == ".xml" || ext == ".xhtml"
	isJSON := ext == ".json"

	// Check if any condition applies to this file before reading it
	needsContent := false
	needsXML := false
	needsJSON := false

	for i := range b.allParsedConditions {
		pc := &b.allParsedConditions[i]
		switch pc.cap {
		case "filecontent":
			if pc.condition.Filecontent.FilePattern != "" {
				if !matchesFilePattern(pc.compiledFilePattern, pc.condition.Filecontent.FilePattern, relPath, baseName) {
					continue
				}
			}
			needsContent = true
		case "xml", "xmlPublicID":
			if isXML {
				needsXML = true
			}
		case "json":
			if isJSON {
				needsJSON = true
			}
		}
	}

	if !needsContent && !needsXML && !needsJSON {
		return
	}

	if ctx.Err() != nil {
		return
	}

	// Clear all existing cache entries for this file before re-processing,
	// so conditions that no longer match don't leave stale entries.
	b.invalidateCacheForFile(originalPath)

	content, err := b.readFileContent(contentPath)
	if err != nil {
		b.log.V(5).Error(err, "failed to read file for cache refresh", "file", contentPath)
		return
	}

	if needsContent {
		b.processFilecontentForFile(originalPath, content, relPath, baseName)
	}

	if needsXML {
		doc, err := b.parseXMLContent(originalPath, content)
		if err != nil {
			b.log.V(5).Error(err, "failed to parse XML for cache refresh", "file", originalPath)
		} else {
			b.processXMLForFile(ctx, originalPath, doc)
		}
	}

	if needsJSON {
		doc, err := jsonquery.Parse(bytes.NewReader(content))
		if err != nil {
			b.log.V(5).Error(err, "failed to parse JSON for cache refresh", "file", originalPath)
		} else {
			b.processJSONForFile(ctx, originalPath, doc)
		}
	}

	b.log.V(5).Info("cache refreshed for working copy", "original", originalPath, "content", contentPath)
}

// processFileForAllCaps runs all applicable pre-compiled condition searches
// against a single file, merging results into the incident cache.
func (b *builtinServiceClient) processFileForAllCaps(ctx context.Context, filePath string) {
	relPath, err := filepath.Rel(b.config.Location, filePath)
	if err != nil {
		relPath = filepath.Base(filePath)
	}
	baseName := filepath.Base(filePath)

	ext := strings.ToLower(filepath.Ext(filePath))
	isXML := ext == ".xml" || ext == ".xhtml"
	isJSON := ext == ".json"

	var content []byte
	contentLoaded := false
	loadContent := func() bool {
		if contentLoaded {
			return content != nil
		}
		contentLoaded = true
		fileInfo, err := os.Stat(filePath)
		if err != nil {
			b.log.V(5).Error(err, "failed to stat file during Prepare", "file", filePath)
			return false
		}
		if fileInfo.Size() > maxCacheFileSize {
			b.log.V(5).Info("skipping large file during Prepare", "file", filePath, "size", fileInfo.Size())
			return false
		}
		content, err = b.readFileContent(filePath)
		if err != nil {
			b.log.V(5).Error(err, "failed to read file during Prepare", "file", filePath)
			content = nil
			return false
		}
		return true
	}

	var xmlDoc *xmlquery.Node
	xmlParsed := false
	var jsonDoc *jsonquery.Node
	jsonParsed := false

	for i := range b.allParsedConditions {
		pc := &b.allParsedConditions[i]
		switch pc.cap {
		case "filecontent":
			if pc.condition.Filecontent.FilePattern != "" {
				if !matchesFilePattern(pc.compiledFilePattern, pc.condition.Filecontent.FilePattern, relPath, baseName) {
					continue
				}
			}
			if !loadContent() {
				return
			}
			pattern := pc.condition.Filecontent.Pattern
			trimmedPattern := strings.TrimPrefix(pattern, "\"")
			if !strings.HasSuffix(trimmedPattern, `\"`) {
				trimmedPattern = strings.TrimSuffix(trimmedPattern, `"`)
			}
			matches := b.searchContentForPattern(filePath, content, trimmedPattern)
			if len(matches) > 0 {
				b.mergeIntoCache(pc.cacheKey, filePath, filecontentMatchesToIncidents(matches))
			}

		case "xml", "xmlPublicID":
			if !isXML {
				continue
			}
			if pc.compiledXPath == nil {
				continue
			}
			if !loadContent() {
				return
			}
			if !xmlParsed {
				xmlParsed = true
				xmlDoc, err = b.parseXMLContent(filePath, content)
				if err != nil {
					b.log.V(5).Error(err, "failed to parse XML during Prepare", "file", filePath)
				}
			}
			if xmlDoc == nil {
				continue
			}
			nodes, err := queryXMLDoc(xmlDoc, pc.compiledXPath)
			if err != nil {
				b.log.V(5).Error(err, "failed to query XML during Prepare", "file", filePath, "cap", pc.cap)
				continue
			}
			if pc.cap == "xml" {
				if len(nodes) == 0 {
					continue
				}
				incidents := make([]provider.IncidentContext, 0, len(nodes))
				for _, node := range nodes {
					incidents = append(incidents, b.xmlNodeToIncident(ctx, filePath, node))
				}
				b.mergeIntoCache(pc.cacheKey, filePath, incidents)
			} else {
				var incidents []provider.IncidentContext
				for _, node := range nodes {
					for _, attr := range node.Attr {
						if attr.Name.Local == "public-id" {
							if pc.compiledRegex != nil && pc.compiledRegex.MatchString(attr.Value) {
								incidents = append(incidents, xmlPublicIDNodeToIncident(filePath, node))
							}
							break
						}
					}
				}
				b.mergeIntoCache(pc.cacheKey, filePath, incidents)
			}

		case "json":
			if !isJSON {
				continue
			}
			if pc.jsonXPath == "" {
				continue
			}
			if !loadContent() {
				return
			}
			if !jsonParsed {
				jsonParsed = true
				jsonDoc, err = jsonquery.Parse(bytes.NewReader(content))
				if err != nil {
					b.log.V(5).Error(err, "failed to parse JSON during Prepare", "file", filePath)
				}
			}
			if jsonDoc == nil {
				continue
			}
			list, err := jsonquery.QueryAll(jsonDoc, pc.jsonXPath)
			if err != nil {
				b.log.V(5).Error(err, "failed to query JSON during Prepare", "file", filePath)
				continue
			}
			if len(list) == 0 {
				continue
			}
			incidents := make([]provider.IncidentContext, 0, len(list))
			for _, node := range list {
				incidents = append(incidents, b.jsonNodeToIncident(ctx, filePath, node))
			}
			b.mergeIntoCache(pc.cacheKey, filePath, incidents)
		}
	}
}

// processFilecontentForFile runs all filecontent conditions against pre-read content.
func (b *builtinServiceClient) processFilecontentForFile(filePath string, content []byte, relPath string, baseName string) {
	for i := range b.allParsedConditions {
		pc := &b.allParsedConditions[i]
		if pc.cap != "filecontent" {
			continue
		}

		if pc.condition.Filecontent.FilePattern != "" {
			if !matchesFilePattern(pc.compiledFilePattern, pc.condition.Filecontent.FilePattern, relPath, baseName) {
				continue
			}
		}

		pattern := pc.condition.Filecontent.Pattern
		trimmedPattern := strings.TrimPrefix(pattern, "\"")
		if !strings.HasSuffix(trimmedPattern, `\"`) {
			trimmedPattern = strings.TrimSuffix(trimmedPattern, `"`)
		}

		matches := b.searchContentForPattern(filePath, content, trimmedPattern)
		if len(matches) == 0 {
			continue
		}

		b.mergeIntoCache(pc.cacheKey, filePath, filecontentMatchesToIncidents(matches))
	}
}

// processXMLForFile runs all xml and xmlPublicID conditions against a pre-parsed XML DOM.
func (b *builtinServiceClient) processXMLForFile(ctx context.Context, filePath string, doc *xmlquery.Node) {
	for i := range b.allParsedConditions {
		pc := &b.allParsedConditions[i]
		if pc.cap != "xml" && pc.cap != "xmlPublicID" {
			continue
		}
		if pc.compiledXPath == nil {
			continue
		}

		nodes, err := queryXMLDoc(doc, pc.compiledXPath)
		if err != nil {
			b.log.V(5).Error(err, "failed to query XML during Prepare", "file", filePath, "cap", pc.cap)
			continue
		}

		if pc.cap == "xml" {
			if len(nodes) == 0 {
				continue
			}
			incidents := make([]provider.IncidentContext, 0, len(nodes))
			for _, node := range nodes {
				incidents = append(incidents, b.xmlNodeToIncident(ctx, filePath, node))
			}
			b.mergeIntoCache(pc.cacheKey, filePath, incidents)
		} else {
			// xmlPublicID: filter by public-id attribute regex
			var incidents []provider.IncidentContext
			for _, node := range nodes {
				for _, attr := range node.Attr {
					if attr.Name.Local == "public-id" {
						if pc.compiledRegex != nil && pc.compiledRegex.MatchString(attr.Value) {
							incidents = append(incidents, xmlPublicIDNodeToIncident(filePath, node))
						}
						break
					}
				}
			}
			b.mergeIntoCache(pc.cacheKey, filePath, incidents)
		}
	}
}

// processJSONForFile runs all json conditions against a pre-parsed JSON DOM.
func (b *builtinServiceClient) processJSONForFile(ctx context.Context, filePath string, doc *jsonquery.Node) {
	for i := range b.allParsedConditions {
		pc := &b.allParsedConditions[i]
		if pc.cap != "json" || pc.jsonXPath == "" {
			continue
		}

		list, err := jsonquery.QueryAll(doc, pc.jsonXPath)
		if err != nil {
			b.log.V(5).Error(err, "failed to query JSON during Prepare", "file", filePath)
			continue
		}
		if len(list) == 0 {
			continue
		}

		incidents := make([]provider.IncidentContext, 0, len(list))
		for _, node := range list {
			incidents = append(incidents, b.jsonNodeToIncident(ctx, filePath, node))
		}
		b.mergeIntoCache(pc.cacheKey, filePath, incidents)
	}
}
