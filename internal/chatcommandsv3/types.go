package chatcommandsv3

type Command interface {
	isCommand()
}

type RunCommand struct {
	Command string
	Args    []string
	Stdin   []byte
	Cwd     string
	Env     map[string]string
	Timeout int
}

func (RunCommand) isCommand() {}

type SendFile struct {
	Filename    string
	Caption     string
	ContentType string
	Content     []byte
}

func (SendFile) isCommand() {}

type SendText struct {
	Text        string
	ContentType string
	Fenced      bool
	Language    string
}

func (SendText) isCommand() {}

type ConfigList struct{}

func (ConfigList) isCommand() {}

type ConfigSet struct {
	Setting string
	Value   string
}

func (ConfigSet) isCommand() {}

type HostbridgeCommand struct {
	SandboxID string
	Command   Command
}

type Runner interface {
	Execute(cmd Command) error
}
