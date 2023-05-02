// This will need a better name, may we want to move it to top level
// Will be used by providers for common interface way of passing in configuration values.
package lib

import (
	"os"

	"github.com/getkin/kin-openapi/openapi3"
	"go.lsp.dev/uri"
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
	FileURI      uri.URI                `yaml:"fileURI"`
	Effort       *int                   `yaml:"effort,omitempty"`
	Variables    map[string]interface{} `yaml:"variables,omitempty"`
	Links        []ExternalLinks        `yaml:"externalLink,omitempty"`
	CodeLocation *Location              `yaml:"location,omitempty"`
}

type Location struct {
	StartPosition Position
	EndPosition   Position
}

type Position struct {
	/*Line defined:
	 * Line position in a document (zero-based).
	 * If a line number is greater than the number of lines in a document, it defaults back to the number of lines in the document.
	 * If a line number is negative, it defaults to 0.
	 */
	Line float64 `json:"line"`

	/*Character defined:
	 * Character offset on a line in a document (zero-based). Assuming that the line is
	 * represented as a string, the `character` value represents the gap between the
	 * `character` and `character + 1`.
	 *
	 * If the character value is greater than the line length it defaults back to the
	 * line length.
	 * If a line number is negative, it defaults to 0.
	 */
	Character float64 `json:"character"`
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
