package llamacppagent

type RefreshContainer struct{}

func RegisterGobTypes(register func(any)) {
	register(RefreshContainer{})
}
