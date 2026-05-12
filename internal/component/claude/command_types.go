package claude

type RefreshContainer struct{}
type StartContainer struct{}
type StopContainer struct{}
type PurgeChat struct{}
type InterruptTurn struct{}
type Status struct{}
type ModelStatus struct{}
type ModelSet struct{ Model string }
type ModelClear struct{}

func RegisterGobTypes(register func(any)) {
	register(RefreshContainer{})
	register(StartContainer{})
	register(StopContainer{})
	register(PurgeChat{})
	register(InterruptTurn{})
	register(Status{})
	register(ModelStatus{})
	register(ModelSet{})
	register(ModelClear{})
}
