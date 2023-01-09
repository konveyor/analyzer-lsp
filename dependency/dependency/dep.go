package dependency

type Dep struct {
	Name     string `json:"name,omitempty"`
	Version  string `json:"version,omitempty"`
	Location string `json:"location,omitempty"`
}
