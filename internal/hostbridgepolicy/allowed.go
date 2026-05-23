package hostbridgepolicy

// AllowedCommand describes one host command that an agent may request through
// hostbridge. It is policy data, not server/runtime machinery, so workspace and
// app config can depend on this package without importing hostbridge/server.
type AllowedCommand struct {
	Name           string            `json:"name"`
	Args           []string          `json:"args"`
	ArgsPattern    string            `json:"args_pattern,omitempty"`
	Dir            string            `json:"dir"`
	Delay          string            `json:"delay"`
	Env            map[string]string `json:"env"`
	AllowExtraArgs bool              `json:"allow_extra_args"`
}
