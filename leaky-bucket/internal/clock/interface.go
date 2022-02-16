package clock

import "time"

// Clock 表示函数的标准库时间的接口
//package clock(本包) 中有以下两种实现:
//第一个是一个实时时钟，它只是封装了时间包的函数。
//第二个是一个模拟时钟，只会在以编程方式调整。
type Clock interface {
	AfterFunc(d time.Duration, f func())
	Now() time.Time
	Sleep(d time.Duration)
}
