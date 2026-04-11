package containerengine

type State string

const (
	StateMissing State = ""
	StateCreated State = "created"
	StateRunning State = "running"
	StateExited  State = "exited"
)

type Mount struct {
	Source   string
	Target   string
	ReadOnly bool
}

type ContainerSpec struct {
	Name         string
	Hostname     string
	Image        string
	Workdir      string
	GPUs         string
	Labels       map[string]string
	Env          []string
	Mounts       []Mount
	SecurityOpts []string
	AddHosts     []string
	Cmd          []string
}
