package dependency

type Dep struct {
	Name     string `json:"name,omitempty"`
	Version  string `json:"version,omitempty"`
	Type     string `json:"type,omitempty"`
	Indirect bool   `json:"indirect,omitempty"`
	SHA      string `json:"sha,omitempty"`
}
