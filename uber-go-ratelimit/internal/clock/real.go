package clock

import "time"

// clock 通过简单地包装 时间包函数(time) 来实现实时时钟。
type clock struct{}

// New 返回一个实时时钟的实例。
func New() Clock {
	return &clock{}
}

// After 返回过去 d 后的时间
func (c *clock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// AfterFunc 使用 time 包中的 AfterFunc
func (c *clock) AfterFunc(d time.Duration, f func()) {
	// TODO 可能会返回 timer 的接口
	time.AfterFunc(d, f)
	//AfterFunc等待持续时间结束，然后在自己的 goroutine 中调用 f 。它返回一个 Timer，可以使用它的 Stop 方法取消调用。
}

// Now 返回现在的时间
func (c *clock) Now() time.Time { return time.Now() }

// Sleep 睡眠 d 的时间
func (c *clock) Sleep(d time.Duration) { time.Sleep(d) }
