package codex

type RefreshContainer struct{}
type StartContainer struct{}
type StopContainer struct{}
type PurgeChat struct{}
type InterruptTurn struct{}
type Status struct{}
type ModelStatus struct{}
type ModelList struct{}
type ModelSet struct {
	Model string
}
type ModelClear struct{}
type ModelEffortStatus struct{}
type ModelEffortList struct{}
type ModelEffortSet struct {
	Effort string
}
type ModelEffortClear struct{}

func RegisterGobTypes(register func(any)) {
	register(RefreshContainer{})
	register(StartContainer{})
	register(StopContainer{})
	register(PurgeChat{})
	register(InterruptTurn{})
	register(Status{})
	register(ModelStatus{})
	register(ModelList{})
	register(ModelSet{})
	register(ModelClear{})
	register(ModelEffortStatus{})
	register(ModelEffortList{})
	register(ModelEffortSet{})
	register(ModelEffortClear{})
}
