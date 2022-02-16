package clock

import (
	"container/heap"
	"sync"
	"time"
)

// Mock 表示只通过编程方式向前移动的模拟时钟。
//当测试基于时间的功能时，它可以比实时时钟更好。
type Mock struct {
	sync.Mutex
	now    time.Time // current time
	timers Timers    // timers
}

// NewMock 返回一个模拟时钟的实例。
//模拟时钟初始化时的当前时间为 时间戳(Unix epoch)。
func NewMock() *Mock {
	return &Mock{now: time.Unix(0, 0)}
}

// Add 将模拟时钟的当前时间向前移动 d 。
//每次只能从一个 goroutine 调用。
func (m *Mock) Add(d time.Duration) {
	m.Lock()
	// Calculate the final time.
	end := m.now.Add(d)

	for len(m.timers) > 0 && m.now.Before(end) {
		t := heap.Pop(&m.timers).(*Timer)
		m.now = t.next
		m.Unlock()
		t.Tick()
		m.Lock()
	}

	m.Unlock()
	//给一个小的缓冲区，以确保其他 goroutines 得到处理。
	nap() //nap 午睡，休息
}

// Timer produces a timer that will emit a time some duration after now.
// Timer 生成一个 Timer，它将在 一段时间(d) 后发出一个时间。
func (m *Mock) Timer(d time.Duration) *Timer {
	ch := make(chan time.Time)
	t := &Timer{
		C:    ch,
		c:    ch,
		mock: m,
		next: m.now.Add(d),
	}
	m.addTimer(t)
	return t
}

func (m *Mock) addTimer(t *Timer) {
	m.Lock()
	defer m.Unlock()
	heap.Push(&m.timers, t)
}

// After produces a channel that will emit the time after a duration passes.
// After 生成一个通道，该通道将在 一个持续时间(d) 过后发出时间。
func (m *Mock) After(d time.Duration) <-chan time.Time {
	return m.Timer(d).C
}

// AfterFunc 等待 duration(d) 结束，然后执行一个函数。
//返回一个可以停止的定时器。
func (m *Mock) AfterFunc(d time.Duration, f func()) *Timer {
	t := m.Timer(d)
	go func() {
		<-t.c
		f()
	}()
	nap()
	return t
}

// Now 现在返回模拟时钟上的当前时间。
func (m *Mock) Now() time.Time {
	m.Lock()
	defer m.Unlock()
	return m.now
}

// Sleep 在模拟时钟上暂停 给定时间(d) 的 goroutine。
//时钟必须向前移动在一个单独的 goroutine。
func (m *Mock) Sleep(d time.Duration) {
	<-m.After(d)
}

// Timer 表示单个事件。
type Timer struct {
	C    <-chan time.Time
	c    chan time.Time
	next time.Time // next tick time
	mock *Mock     // mock clock
}

// Next 进入下一个事件
func (t *Timer) Next() time.Time { return t.next }

func (t *Timer) Tick() {
	select {
	case t.c <- t.next:
	default:
	}
	nap()
}

// nap 暂时休眠，以便其他goroutines可以处理。
func nap() { time.Sleep(1 * time.Millisecond) }
