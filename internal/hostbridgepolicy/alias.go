package hostbridgepolicy

type AliasInstructionVisibility string

const (
	AliasInstructionVisible AliasInstructionVisibility = ""
	AliasInstructionHidden  AliasInstructionVisibility = "hidden"
)

// Alias describes one host command that an agent may request through
// hostbridge. It is policy data, not server/runtime machinery, so workspace and
// app config can depend on this package without importing hostbridge/server.
type Alias struct {
	Name           string                     `json:"name"`
	Args           []string                   `json:"args"`
	ArgsPattern    string                     `json:"args_pattern,omitempty"`
	Subcommands    map[string]AliasSubcommand `json:"subcommands,omitempty"`
	Dir            string                     `json:"dir"`
	Delay          string                     `json:"delay"`
	Env            map[string]string          `json:"env"`
	AllowExtraArgs bool                       `json:"allow_extra_args"`
	// InstructionVisibility controls whether this allowed alias is shown in
	// agent-facing prompt synopses. It never affects execution authority.
	InstructionVisibility AliasInstructionVisibility `json:"instruction_visibility,omitempty"`
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
