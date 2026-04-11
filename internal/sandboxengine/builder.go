package sandboxengine

type Builder struct {
	sbx *Sandbox
}

func NewBuilder(manager Manager, name string) Builder {
	return Builder{sbx: manager.NewSandbox(name)}
}

func (b Builder) WorkspaceDir(v string) Builder {
	b.sbx.WorkspaceDir = v
	return b
}

func (b Builder) ProfileDir(v string) Builder {
	b.sbx.ProfileDir = v
	return b
}

func (b Builder) ContainerWorkspace(v string) Builder {
	b.sbx.ContainerWorkspace = v
	return b
}

func (b Builder) ContainerHome(v string) Builder {
	b.sbx.ContainerHome = v
	return b
}

func (b Builder) DeveloperInstructions(v string) Builder {
	b.sbx.DeveloperInstructions = v
	return b
}

func (b Builder) Hostname(v string) Builder {
	b.sbx.Hostname = v
	return b
}

func (b Builder) Image(v string) Builder {
	b.sbx.Image = v
	return b
}

func (b Builder) Workdir(v string) Builder {
	b.sbx.Workdir = v
	return b
}

func (b Builder) GPUs(v string) Builder {
	b.sbx.GPUs = v
	return b
}

func (b Builder) Labels(v map[string]string) Builder {
	b.sbx.Labels = v
	return b
}

func (b Builder) Env(v []string) Builder {
	b.sbx.Env = v
	return b
}

func (b Builder) Mounts(v []Mount) Builder {
	b.sbx.Mounts = v
	return b
}

func (b Builder) SecurityOpts(v []string) Builder {
	b.sbx.SecurityOpts = v
	return b
}

func (b Builder) AddHosts(v []string) Builder {
	b.sbx.AddHosts = v
	return b
}

func (b Builder) Cmd(v []string) Builder {
	b.sbx.Cmd = v
	return b
}

func (b Builder) Build() *Sandbox {
	return b.sbx
}
