package hostbridge

type Request struct {
	Command string
	Args    []string
	Stdin   []byte
	Cwd     string
	Env     map[string]string
	Timeout int
}

type StreamKind uint8

const (
	StreamStdout StreamKind = 1
	StreamStderr StreamKind = 2
	StreamExit   StreamKind = 3
	StreamError  StreamKind = 4
)

type Frame struct {
	Kind     StreamKind
	Data     []byte
	ExitCode int
	Message  string
}
