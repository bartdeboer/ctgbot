package claude

type RefreshContainer struct{}
type StartContainer struct{}
type StopContainer struct{}
type PurgeChat struct{}
type InterruptTurn struct{}
type Status struct{}

func RegisterGobTypes(register func(any)) {
	register(RefreshContainer{})
	register(StartContainer{})
	register(StopContainer{})
	register(PurgeChat{})
	register(InterruptTurn{})
	register(Status{})
}
