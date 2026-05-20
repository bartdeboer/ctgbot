package sandboxengine

type Builder struct {
	spec *SandboxSpec
}

func NewBuilder(name string) Builder {
	return Builder{spec: &SandboxSpec{Name: name}}
}

func (b Builder) InteractiveInterruptEnabled(v bool) Builder {
	b.spec.InteractiveInterruptEnabled = v
	return b
}

func (b Builder) WorkspaceDir(v string) Builder {
	b.spec.WorkspaceDir = v
	return b
}

func (b Builder) ProfileDir(v string) Builder {
	b.spec.ProfileDir = v
	return b
}

func (b Builder) ContainerWorkspace(v string) Builder {
	b.spec.ContainerWorkspace = v
	return b
}

func (b Builder) ContainerHome(v string) Builder {
	b.spec.ContainerHome = v
	return b
}

func (b Builder) DeveloperInstructions(v string) Builder {
	b.spec.DeveloperInstructions = v
	return b
}

func (b Builder) Hostname(v string) Builder {
	b.spec.Hostname = v
	return b
}

func (b Builder) Image(v string) Builder {
	b.spec.Image = v
	return b
}

func (b Builder) Workdir(v string) Builder {
	b.spec.Workdir = v
	return b
}

func (b Builder) UserMode(v string) Builder {
	b.spec.UserMode = v
	return b
}

func (b Builder) GPUs(v string) Builder {
	b.spec.GPUs = v
	return b
}

func (b Builder) Labels(v map[string]string) Builder {
	b.spec.Labels = v
	return b
}

func (b Builder) Env(v []string) Builder {
	b.spec.Env = v
	return b
}

func (b Builder) Mounts(v []Mount) Builder {
	b.spec.Mounts = v
	return b
}

func (b Builder) SecurityOpts(v []string) Builder {
	b.spec.SecurityOpts = v
	return b
}

func (b Builder) AddHosts(v []string) Builder {
	b.spec.AddHosts = v
	return b
}

func (b Builder) Cmd(v []string) Builder {
	b.spec.Cmd = v
	return b
}

func (b Builder) RecreateStopped(v bool) Builder {
	b.spec.RecreateStopped = v
	return b
}

func (b Builder) Build() *SandboxSpec {
	return b.spec
}
