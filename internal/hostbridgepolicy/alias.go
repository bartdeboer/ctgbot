package hostbridgepolicy

// Alias describes one host command that an agent may request through
// hostbridge. It is policy data, not server/runtime machinery, so workspace and
// app config can depend on this package without importing hostbridge/server.
type Alias struct {
	Name        string                     `json:"name"`
	Args        []string                   `json:"args"`
	ArgsPattern string                     `json:"args_pattern,omitempty"`
	Subcommands map[string]AliasSubcommand `json:"subcommands,omitempty"`
	Dir         string                     `json:"dir"`
	// AllowedCWDs lists exact host directories the caller may select with a
	// leading --cwd <path> pair. A sandbox caller may name the corresponding
	// runtime workspace path; Hostbridge translates it before matching. The pair
	// is consumed before invocation. AllowedCWDs cannot be combined with Dir.
	AllowedCWDs    []string          `json:"allowed_cwds,omitempty"`
	Delay          string            `json:"delay"`
	Env            map[string]string `json:"env"`
	AllowExtraArgs bool              `json:"allow_extra_args"`
	// SerializationKey places this alias in one process-local execution lane.
	// Aliases with the same non-empty key execute one at a time.
	SerializationKey string `json:"serialization_key,omitempty"`
	// StdinMaxBytes opts this alias into piped stdin and bounds the accepted
	// payload. Zero denies non-empty stdin.
	StdinMaxBytes int64 `json:"stdin_max_bytes,omitempty"`
}

// AliasSubcommand describes one named operation under an Alias.
//
// If Args is empty, the subcommand name itself is appended after the parent
// command's Args. That keeps simple git-style aliases compact:
//
//	"subcommands": {"fetch": {}, "pull": {}, "status": {}}
//
// For custom expansions, set Args explicitly.
type AliasSubcommand struct {
	Args           []string `json:"args"`
	ArgsPattern    string   `json:"args_pattern,omitempty"`
	AllowExtraArgs bool     `json:"allow_extra_args"`
}
