// This will need a better name, may we want to move it to top level
// Will be used by providers for common interface way of passing in configuration values.
package lib

import (
	"os"

	"gopkg.in/yaml.v2"
)

var builtinConfig = Config{
	Name:     "builtin",
	Location: ".",
}

type Config struct {
	Name string `yaml:"name,omitempty"`

	// This is the location of the code base that the
	// Provider will be responisble for parsing
	Location string `yaml:"location,omitempty"`

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
	configs = append(configs, builtinConfig)

	return configs, nil

}

type ProviderEvaluateResponse struct {
	Passed              bool
	ConditionHitContext []map[string]string
	TemplateContext     map[string]interface{}
}
