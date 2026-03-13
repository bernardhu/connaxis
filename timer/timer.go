package timer

type ITimer interface {
	GetID() uint64
	GetExpriate() int64
	SetExpriate(int64)
	OnTimeOut()
	SetSlot(int)
	GetSlot() int
}

type ITimerWheel interface {
	GetInterval() int64
	GetFD() int
	SetFD(int)
	OnTimeOut()
	AddTimer(t ITimer) bool
	DelTimer(t ITimer)
	RefreshTimer(t ITimer) bool
}

type ITimeSource interface {
	AddTimerWheel(tw ITimerWheel) (int, error)
	DelTimerWheel(tw ITimerWheel)
	Stop()
}
