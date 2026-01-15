package labels

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/engine/labels"
)

const (
	JavaDepSourceInternal                      = "internal"
	JavaDepSourceOpenSource                    = "open-source"
	ProviderSpecificConfigOpenSourceDepListKey = "depOpenSourceLabelsFile"
	ProviderSpecificConfigExcludePackagesKey   = "excludePackages"
)

const (
	// Dep source label is a label key that any provider can use, to label the dependencies as coming from a particular source.
	// Examples from java are: open-source and internal. A provider can also have a user provide file that will tell them which
	// depdendencies to label as this value. This label will be used to filter out these dependencies from a given analysis
	DepSourceLabel   = "konveyor.io/dep-source"
	DepExcludeLabel  = "konveyor.io/exclude"
	DepLanguageLabel = "konveyor.io/language"
)

type openSourceLabels bool

func (o openSourceLabels) GetLabels() []string {
	return []string{
		labels.AsString(DepSourceLabel, JavaDepSourceOpenSource),
	}
}

type Labeler interface {
	AddLabels(string, bool) []string
	HasLabel(string) bool
}

type labeler struct {
	depToLabels map[string]*depLabelItem
}

type depLabelItem struct {
	r      *regexp.Regexp
	labels map[string]any
}

func GetOpenSourceLabeler(config map[string]any, log logr.Logger) (Labeler, error) {
	depToLabels, err := initOpenSourceDepLabels(config, log)
	if err != nil {
		return nil, err
	}
	return &labeler{
		depToLabels: depToLabels,
	}, nil
}

func GetExcludeDepLabels(config map[string]any, log logr.Logger, l Labeler) (Labeler, error) {
	la, ok := l.(*labeler)
	if !ok {
		return nil, fmt.Errorf("labeler must be already created")
	}

	depToLabels, err := initExcludeDepLabels(config, la.depToLabels, log)
	if err != nil {
		return nil, err
	}

	return &labeler{depToLabels: depToLabels}, nil

}

func (l *labeler) HasLabel(key string) bool {
	_, ok := l.depToLabels[key]
	return ok
}

// addLabels adds some labels (open-source/internal and java) to the dependencies. The openSource argument can be used
// in cased it was already determined that the dependency is open source by any other means (ie by inferring the groupId)
func (l *labeler) AddLabels(depName string, openSource bool) []string {
	m := map[string]any{}
	for _, d := range l.depToLabels {
		if d.r.Match([]byte(depName)) {
			for label := range d.labels {
				m[label] = nil
			}
		}
	}
	s := []string{}
	for k := range m {
		s = append(s, k)
	}
	// if open source label is not found and we don't know if it's open source yet, qualify the dep as being internal by default
	_, openSourceLabelFound := m[labels.AsString(DepSourceLabel, JavaDepSourceOpenSource)]
	_, internalSourceLabelFound := m[labels.AsString(DepSourceLabel, JavaDepSourceInternal)]
	if openSourceLabelFound || openSource {
		if !openSourceLabelFound {
			s = append(s, labels.AsString(DepSourceLabel, JavaDepSourceOpenSource))
		}
		if internalSourceLabelFound {
			delete(m, labels.AsString(DepSourceLabel, JavaDepSourceInternal))
		}
	} else {
		if !internalSourceLabelFound {
			s = append(s, labels.AsString(DepSourceLabel, JavaDepSourceInternal))
		}
	}
	s = append(s, labels.AsString(DepLanguageLabel, "java"))
	return s
}

// initOpenSourceDepLabels reads user provided file that has a list of open source
// packages (supports regex) and loads a map of patterns -> labels for easy lookup
func initOpenSourceDepLabels(providerSpecificConfig map[string]any, log logr.Logger) (map[string]*depLabelItem, error) {
	var ok bool
	var v any
	if v, ok = providerSpecificConfig[ProviderSpecificConfigOpenSourceDepListKey]; !ok {
		log.V(7).Info("Did not find open source dep list.")
		return nil, nil
	}

	var filePath string
	if filePath, ok = v.(string); !ok {
		return nil, fmt.Errorf("unable to determine filePath from open source dep list")
	}

	fileInfo, err := os.Stat(filePath)
	if err != nil {
		//TODO(shawn-hurley): consider wrapping error with value
		return nil, err
	}

	if fileInfo.IsDir() {
		return nil, fmt.Errorf("open source dep list must be a file, not a directory")
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	items, err := loadDepLabelItems(file, labels.AsString(DepSourceLabel, JavaDepSourceOpenSource), nil)
	return items, nil
}

// initExcludeDepLabels reads user provided list of excluded packages
// and initiates label lookup for them
func initExcludeDepLabels(providerSpecificConfig map[string]any, depToLabels map[string]*depLabelItem, log logr.Logger) (map[string]*depLabelItem, error) {
	var ok bool
	var v any
	if v, ok = providerSpecificConfig[ProviderSpecificConfigExcludePackagesKey]; !ok {
		log.V(7).Info("did not find exclude packages list")
		return depToLabels, nil
	}
	excludePackages, ok := v.([]string)
	if !ok {
		return nil, fmt.Errorf("%s config must be a list of packages to exclude", ProviderSpecificConfigExcludePackagesKey)
	}
	items, err := loadDepLabelItems(strings.NewReader(strings.Join(excludePackages, "\n")), DepExcludeLabel, depToLabels)
	if err != nil {
		return nil, err
	}
	return items, nil
}

// loadDepLabelItems reads list of patterns from reader and appends given
// label to the list of labels for the associated pattern
func loadDepLabelItems(r io.Reader, label string, depToLabels map[string]*depLabelItem) (map[string]*depLabelItem, error) {
	depToLabelsItems := map[string]*depLabelItem{}
	if depToLabels != nil {
		depToLabelsItems = depToLabels
	}
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		pattern := scanner.Text()
		r, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("unable to create regexp for string: %v", pattern)
		}
		//Make sure that we are not adding duplicates
		if _, found := depToLabelsItems[pattern]; !found {
			depToLabelsItems[pattern] = &depLabelItem{
				r: r,
				labels: map[string]any{
					label: nil,
				},
			}
		} else {
			if depToLabelsItems[pattern].labels == nil {
				depToLabelsItems[pattern].labels = map[string]any{}
			}
			depToLabelsItems[pattern].labels[label] = nil
		}
	}
	return depToLabelsItems, nil
}

func CanRestrictSelector(depLabelSelector string) (bool, error) {
	selector, err := labels.NewLabelSelector[*openSourceLabels](depLabelSelector, nil)
	if err != nil {
		return false, err
	}
	if selector == nil {
		return false, err
	}
	matcher := openSourceLabels(true)
	return selector.Matches(&matcher)
}
