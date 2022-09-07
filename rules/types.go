package rules

type Configuration struct {
	ProjectLocation string
}

type Rule struct {
	*ImportRule `json:","`
}

type ImportRule struct {
	GoImportRule     *GoImportRule     `json:"go,omitempty"`
	PythonImportRule *PythonImportRule `json:"python,omitempty"`
	JavaImportRule   *JavaImportRule   `json:"java,omitempty"`
}

type GoImportRule struct {
	Import  string `json:"import,omitempty"`
	Message string `json:"message,omitempty"`
}

type PythonImportRule struct {
	Import  string `json:"import,omitempty"`
	Message string `json:"message,omitempty"`
}

type JavaImportRule struct {
	Import  string `json:"import,omitempty"`
	Message string `json:"message,omitempty"`
}
