// This will need a better name, may we want to move it to top level
// Will be used by providers for common interface way of passing in configuration values.
package lib

import (
	"os"

	"github.com/getkin/kin-openapi/openapi3"
	"gopkg.in/yaml.v2"
)

var builtinConfig = Config{
	Name:     "builtin",
	Location: ".",
}

type Capability struct {
	Name            string
	TemplateContext openapi3.SchemaRef
}

type Config struct {
	Name string `yaml:"name,omitempty"`

	// This is the location of the code base that the
	// Provider will be responisble for parsing
	Location string `yaml:"location,omitempty"`

	// This is the path to look for the dependencies for the project.
	// It is relative to the Location
	DependencyPath string `yaml:"dependencyPath,omitempty"`

	BinaryLocation string `yaml:"binaryLocation,omitempty"`

	// This will have to be defined for each provider
	ProviderSpecificConfig map[string]string `yaml:"providerSpecificConfig,omitempty"`
}

func GetConfig(filepath string) ([]Config, error) {

	content, err := os.ReadFile(filepath)
	if err != nil {
		return nil, err
	}

	configs := []Config{}

	err = yaml.Unmarshal(content, &configs)
	if err != nil {
		return nil, err
	}
	foundBuiltin := false
	for _, c := range configs {
		if c.Name == builtinConfig.Name {
			foundBuiltin = true
		}
	}
	if !foundBuiltin {
		configs = append(configs, builtinConfig)
	}

	return configs, nil

}

type ProviderEvaluateResponse struct {
	Matched         bool                   `yaml:"matched"`
	Incidents       []IncidentContext      `yaml:"incidents"`
	TemplateContext map[string]interface{} `yaml:"templateContext"`
}
type IncidentContext struct {
	FileURI string                 `yaml:"fileURI"`
	Effort  *int                   `yaml:"effort,omitempty"`
	Extras  map[string]interface{} `yaml:"extras,omitempty"`
	Links   []ExternalLinks        `yaml:"externalLink,omitempty"`
}

type ExternalLinks struct {
	URL   string `yaml:"url"`
	Title string `yaml:"title"`
}

type ChainTemplate struct {
	Filepaths []string               `yaml:"filepaths"`
	Extras    map[string]interface{} `yaml:"extras"`
}

type ProviderContext struct {
	Tags     map[string]interface{}   `yaml:"tags"`
	Template map[string]ChainTemplate `yaml:"template"`
}

func HasCapability(caps []Capability, name string) bool {
	for _, cap := range caps {
		if cap.Name == name {
			return true
		}
	}
	return false
}
