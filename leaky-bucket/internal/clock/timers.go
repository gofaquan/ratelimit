package clock

// Timers 表示一个可排序的计时器列表，我的理解为 一个双向链表。
type Timers []*Timer

// Len 返回列表的长度
func (ts Timers) Len() int { return len(ts) }

// Swap 元素交换
func (ts Timers) Swap(i, j int) {
	ts[i], ts[j] = ts[j], ts[i]
}

// Less 把 ts[i] ~ ts[j] 从双向链表中去除
func (ts Timers) Less(i, j int) bool {
	return ts[i].Next().Before(ts[j].Next())
}

// Push 加入一个元素到最列表中，类似 元素的入栈
func (ts *Timers) Push(t interface{}) {
	*ts = append(*ts, t.(*Timer))
}

// Pop 去除一个列表元素，类似 弹出栈顶元素
func (ts *Timers) Pop() interface{} {
	t := (*ts)[len(*ts)-1]
	*ts = (*ts)[:len(*ts)-1]
	return t
}
