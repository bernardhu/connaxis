package timer

import (
	"sync"
	"sync/atomic"
)

// TimeWheel 时间轮
type TimeWheel struct {
	id       int
	interval int64
	slotNum  int // 槽数量
	fd       int

	slots []*TimeWheelSlot // 时间轮槽

	currentPos int32
}

type TimeWheelSlot struct {
	slot      uint32
	seq       uint32
	mu        sync.RWMutex
	container map[uint64]ITimer
}

// New 创建时间轮
func NewTimeWheel(interval int64, id, slotNum int) *TimeWheel {
	if interval <= 0 || slotNum <= 0 {
		return nil
	}
	tw := &TimeWheel{
		id:         id,
		interval:   interval,
		slots:      make([]*TimeWheelSlot, slotNum),
		slotNum:    slotNum,
		currentPos: 0,
	}

	for i := 0; i < tw.slotNum; i++ {
		tws := new(TimeWheelSlot)
		tws.slot = uint32(i)
		tws.seq = 0
		tws.container = make(map[uint64]ITimer)
		tw.slots[i] = tws
	}

	return tw
}

func (tw *TimeWheel) GetInterval() int64 {
	return tw.interval
}

func (tw *TimeWheel) GetFD() int {
	return tw.fd
}
func (tw *TimeWheel) SetFD(fd int) {
	tw.fd = fd
}

func (tw *TimeWheel) OnTimeOut() {
	pos := atomic.LoadInt32(&tw.currentPos)
	tws := tw.slots[pos%int32(tw.slotNum)]
	tws.mu.Lock()
	m := tws.container
	tws.container = make(map[uint64]ITimer)
	tws.mu.Unlock()

	for k, t := range m {
		//wrapper.Debugf("OnTimeOut tw:%d timer:%d cur:%d", tw.id, t.GetID(), tw.currentPos)
		go t.OnTimeOut()
		delete(m, k)
	}

	atomic.AddInt32(&tw.currentPos, 1)
}

func (tw *TimeWheel) AddTimer(t ITimer) bool {
	expriate := t.GetExpriate()
	if expriate < 0 {
		//wrapper.Debugf("expraite:%d less than 0", expriate)
		return false
	}

	//todo 多层级
	if expriate > tw.interval*int64(tw.slotNum) {
		//wrapper.Debugf("currently not support multiple hierarchy timewheel want:%d max:%d", expriate, tw.interval*int64(tw.slotNum))
		return false
	}

	slot := t.GetSlot()
	if slot >= 0 && slot < tw.slotNum {
		tws := tw.slots[slot]
		tws.mu.Lock()
		m := tw.slots[slot]
		delete(m.container, t.GetID())
		tws.mu.Unlock()
	}

	pos := int(atomic.LoadInt32(&tw.currentPos)) % tw.slotNum
	slot = (pos + int(expriate/tw.interval)) % tw.slotNum
	tws := tw.slots[slot]
	tws.mu.Lock()
	m := tw.slots[slot]
	m.container[t.GetID()] = t
	tws.mu.Unlock()

	t.SetSlot(slot)

	//wrapper.Debugf("tw:%d timer:%d added to slot:%d exp:%d, cur:%d", tw.id, t.GetID(), slot, expriate, tw.currentPos)

	return true
}

func (tw *TimeWheel) DelTimer(t ITimer) {
	slot := t.GetSlot()
	tws := tw.slots[slot]
	tws.mu.Lock()
	m := tw.slots[slot]
	delete(m.container, t.GetID())
	tws.mu.Unlock()
	//wrapper.Debugf("timer:%d removed from slot:%d", t.GetID(), slot)
}

func (tw *TimeWheel) DelTimerByID(slot int, id uint64) bool {
	if slot >= tw.slotNum {
		return false
	}

	tws := tw.slots[slot]
	tws.mu.Lock()
	defer tws.mu.Unlock()
	if tws.container[id] != nil {
		delete(tws.container, id)
		return true
	}

	return false

	//wrapper.Debugf("timer:%d removed from slot:%d", t.GetID(), slot)
}

func (tw *TimeWheel) RefreshTimer(t ITimer) bool {
	expriate := t.GetExpriate()
	slot := t.GetSlot()

	if slot == -1 {
		tw.AddTimer(t)
		return true
	}

	pos := int(atomic.LoadInt32(&tw.currentPos)) % tw.slotNum
	tgt := (pos + int(expriate/tw.interval)) % tw.slotNum
	if tgt != slot {
		tw.DelTimer(t)
		tw.AddTimer(t)
	} else {
		//wrapper.Debugf("tw:%d timer:%d slots till in:%d", tw.id, t.GetID(), slot)
	}

	return true
}
