package tokenBucket

import (
	"math"
	"strconv"
	"sync"
	"time"
)

//虽说是令牌桶，但是我们没有必要真的去生成令牌放到桶里，
//我们只需要每次来取令牌的时候计算一下，当前是否有足够的令牌就可以了，
//具体的计算方式可以总结为下面的公式：
//当前令牌数 = 上一次剩余的令牌数 + (本次取令牌的时刻-上一次取令牌的时刻 )/放置令牌的时间间隔 * 每次放置的令牌数

// Bucket 表示以预定速率填充的令牌桶。
// Bucket 上的方法可以并发调用。
type Bucket struct {
	clock Clock

	// startTime 当第一次创建以及令牌开始进出时 保存桶的时间。
	startTime time.Time

	// capacity 表示桶的总容量。
	capacity int64

	// quantum 表示每次填充了多少令牌进桶。
	quantum int64

	// fillInterval 表示每次填充的时间间隔。
	fillInterval time.Duration

	//用锁保护下面的两个字段
	mu sync.Mutex

	// availableTokens holds the number of available
	// tokens as of the associated latestTick.
	// It will be negative when there are consumers
	// waiting for tokens.
	// availableTokens 保存从 上次取令牌 到 现在 可用数量的令牌
	//当有消费者们在等待令牌时，将是负的
	availableTokens int64

	// latestTick holds the latest tick for which
	// we know the number of tokens in the bucket.
	//latestTick 持有最新的我们知道桶中的令牌数。
	latestTick int64
}

// NewBucket 创建指定 填充速率 和 容量大小 的满令牌桶，参数均要为正
func NewBucket(fillInterval time.Duration, capacity int64) *Bucket {
	return NewBucketWithClock(fillInterval, capacity, nil)
}

// NewBucketWithClock 和 NewBucket 是一样的，只是加入了一个可测试的时钟接口。
func NewBucketWithClock(fillInterval time.Duration, capacity int64, clock Clock) *Bucket {
	return NewBucketWithQuantumAndClock(fillInterval, capacity, 1, clock)
}

// rateMargin 指定允许的误差。1%似乎是合理的。
const rateMargin = 0.01

// NewBucketWithRate 创建填充速度为指定速率和容量大小的令牌桶
// NewBucketWithRate(0.1, 200) 表示每秒填充 20 (0.1 * 200) 个令牌
func NewBucketWithRate(rate float64, capacity int64) *Bucket {
	return NewBucketWithRateAndClock(rate, capacity, nil)
}

// NewBucketWithRateAndClock 与 NewBucketWithRate 相同，但加入了一个可测试时钟接口。
func NewBucketWithRateAndClock(rate float64, capacity int64, clock Clock) *Bucket {
	//每次循环使用相同的桶 (tb)保存分配额。
	//由 NewBucketWithRate 函数知，按秒填充，每次填充 rate * capacity 个令牌,无消耗则 1 / rate 秒后填满
	tb := NewBucketWithQuantumAndClock(1, capacity, 1, clock)

	//待完善,按我的理解应该是通过下面的循环计算方式找到最适合的 quantum fillInterval
	//使得 capacity / quantum * fillInterval  = 1 / rate
	for quantum := int64(1); quantum < 1<<50; quantum = nextQuantum(quantum) {
		fillInterval := time.Duration(1e9 * float64(quantum) / rate)
		if fillInterval <= 0 {
			continue
		}
		tb.fillInterval = fillInterval
		tb.quantum = quantum
		//在误差内就返回桶
		if diff := math.Abs(tb.Rate() - rate); diff/rate <= rateMargin {
			return tb
		}
	}
	//超过误差允许范围，panic!
	panic("当 rate = " + strconv.FormatFloat(rate, 'g', -1, 64) +
		" 时，找不到合适的 quantum 来满足填充条件")
}

// nextQuantum 返回一个大于 q 的数。
//我们以指数方式增长，但速度缓慢，
//所以我们得到一个较低的数字。
func nextQuantum(q int64) int64 {
	//第一步: q 通过乘11再除10来变大
	q1 := q * 11 / 10
	//第二步: 要是第一步后 q 仍然 = q ，则 q = q + 1 确保变大了
	if q1 == q {
		q1++
	}
	return q1
}

// NewBucketWithQuantum 类似于 NewBucket，但可以指定每次填充的令牌量的多少
func NewBucketWithQuantum(fillInterval time.Duration, capacity, quantum int64) *Bucket {
	return NewBucketWithQuantumAndClock(fillInterval, capacity, quantum, nil)
}

// NewBucketWithQuantumAndClock 类似于 NewBucketWithQuantum，
//加入了一个时钟参数，允许客户端伪造传递时间。如果 clock为 nil，则使用系统时钟。
func NewBucketWithQuantumAndClock(fillInterval time.Duration, capacity, quantum int64, clock Clock) *Bucket {
	//判断条件，不满足则添加
	if clock == nil { //clock 为空，则新建一个
		clock = realClock{}
	}
	if fillInterval <= 0 {
		panic("token bucket fill interval is not > 0")
	} //不允许填充间隔为负
	if capacity <= 0 {
		panic("token bucket capacity is not > 0")
	} //不允许容量为负
	if quantum <= 0 {
		panic("token bucket quantum is not > 0")
	} //不允许每次填充令牌为负数

	//满足上述条件后，返回合理的桶
	return &Bucket{
		clock:           clock,
		startTime:       clock.Now(),
		latestTick:      0,
		fillInterval:    fillInterval,
		capacity:        capacity,
		quantum:         quantum,
		availableTokens: capacity,
	}
}

// Wait 取令牌（阻塞）
// Wait 获取桶中令牌数，等待直到有令牌可用。
func (tb *Bucket) Wait(count int64) {
	if d := tb.Take(count); d > 0 {
		tb.clock.Sleep(d)
	}
}

// WaitMaxDuration 取令牌（阻塞）
// WaitMaxDuration is like Wait except that it will
// only take tokens from the bucket if it needs to wait
// for no greater than maxWait. It reports whether
// any tokens have been removed from the bucket
// If no tokens have been removed, it returns immediately.
// WaitMaxDuration 类似于 Wait，它会获取桶中令牌数，
//如果它需要等待的时间 不大于 maxWait才会获取令牌。
//它检查是否有令牌已经从桶中消耗
//如果没有令牌被消耗，它立即返回。
func (tb *Bucket) WaitMaxDuration(count int64, maxWait time.Duration) bool {
	d, ok := tb.TakeMaxDuration(count, maxWait)
	if d > 0 {
		tb.clock.Sleep(d)
	}
	return ok
}

const infinityDuration time.Duration = 0x7fffffffffffffff // 2^63 - 1

// Take 取令牌（非阻塞）
// Take takes count tokens from the bucket without blocking. It returns
// the time that the caller should wait until the tokens are actually
// available.
// Note that if the request is irrevocable - there is no way to return
// tokens to the bucket once this method commits us to taking them.
// Take 从桶中取走 count 个令牌，且不会阻塞。它返回调用者应该等待的时间，直到令牌可用。
//注意，如果请求是不可撤回的 - 不能返回此方法使用的令牌。
func (tb *Bucket) Take(count int64) time.Duration {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	d, _ := tb.take(tb.clock.Now(), count, infinityDuration) //infinityDuration 这么大 ，我认为默认一直等待
	return d
}

// TakeMaxDuration 最多等maxWait时间取token
// TakeMaxDuration is like Take, except that
// it will only take tokens from the bucket if the wait
// time for the tokens is no greater than maxWait.
//
// If it took longer than maxWait for the tokens
// to become available, it does nothing and reports false,
// otherwise it returns the time that the caller should
// wait until the tokens are actually available, and reports
// true.
// TakeMaxDuration 类似于 Take，
//只有当等待令牌的时间不大于 maxWait，将可以从桶中获取令牌。
//返回等待直到令牌实际可用的时间和 true。
//如果它需要比 maxWait 更长时间使令牌变成可用， 它将返回 false，
func (tb *Bucket) TakeMaxDuration(count int64, maxWait time.Duration) (time.Duration, bool) {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return tb.take(tb.clock.Now(), count, maxWait)
}

// TakeAvailable 取令牌（非阻塞）
// TakeAvailable takes up to count immediately available tokens from the
// bucket. It returns the number of tokens removed, or zero if there are
// no available tokens. It does not block.
// TakeAvailable 占用可用的令牌桶。它返回被使用的令牌的数量或者 0
//如果没有可用的令牌。它也不会阻塞。
func (tb *Bucket) TakeAvailable(count int64) int64 {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return tb.takeAvailable(tb.clock.Now(), count)
}

// takeAvailable 是 TakeAvailable 的内部版本
//它接受当前时间作为参数，以方便测试。
func (tb *Bucket) takeAvailable(now time.Time, count int64) int64 {
	if count <= 0 { // 取走 0 个令牌
		return 0 // 表明立即取走
	}
	tb.adjustavailableTokens(tb.currentTick(now)) //调整令牌数

	if tb.availableTokens <= 0 { //发现无可以令牌
		return 0
	}
	if count > tb.availableTokens { //现有令牌不够取
		count = tb.availableTokens //能取多少取多少
	}
	tb.availableTokens -= count // 可用令牌 = 可用令牌 - 需要的令牌数
	return count                //返回取走令牌数
}

// Available returns the number of available tokens. It will be negative
// when there are consumers waiting for tokens. Note that if this
// returns greater than zero, it does not guarantee that calls that take
// tokens from the buffer will succeed, as the number of available
// tokens could have changed in the meantime. This method is intended
// primarily for metrics reporting and debugging.
// Available 返回可用令牌的数量。
//当有消费者在等待令牌时，结果将是负的
//注意如果返回大于 0 的值，它不能保证从缓冲区取令牌成功，
//因为可用的令牌数量可能在此期间发生了变化。
//这个方法的目的是主要用于度量报告和调试。
func (tb *Bucket) Available() int64 {
	return tb.available(tb.clock.Now())
}

// available 是 Available 的内部版本-它加入以当前时间为一个参数，使易于测试。
func (tb *Bucket) available(now time.Time) int64 {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.adjustavailableTokens(tb.currentTick(now))
	return tb.availableTokens
}

// Capacity 返回创建桶时使用的容量。
func (tb *Bucket) Capacity() int64 {
	return tb.capacity
}

// Rate 返回桶的填充率，单位为 令牌/秒。
func (tb *Bucket) Rate() float64 {
	//一次 quantum 个，fillInterval 秒，速率是quantum/fillInterval
	return 1e9 * float64(tb.quantum) / float64(tb.fillInterval)
}

// take 是 Take 的内部版本-它加入当前时间作为 一个参数，使易于测试。
func (tb *Bucket) take(now time.Time, count int64, maxWait time.Duration) (time.Duration, bool) {
	//取走负数个令牌
	if count <= 0 {
		return 0, true //表明过了 0 ns 立即成功，能取走
	}

	tick := tb.currentTick(now)    // 走了 tick 个 时间间隔(fillInterval)
	tb.adjustavailableTokens(tick) //调整令牌数量

	avail := tb.availableTokens - count // 可用令牌 - 要的令牌数
	//1. 令牌足够
	if avail >= 0 {
		tb.availableTokens = avail // 可用令牌  = 可用令牌 - 要的令牌数
		return 0, true             //表明过了 0 ns 立即成功，能取走
	}

	//2.令牌不足
	//将缺失的令牌四舍五入到最近的 quantum 的倍数
	//令牌将无法使用，直到过了 上方的时间间隔倍数 的时间，使得令牌足够
	// endTick = 令牌数 达到 能够取走的数目(count) 的 时间间隔总数
	endTick := tick + (-avail+tb.quantum-1)/tb.quantum
	// 等待结束的时间 = endTime = startTime + 间隔数time.Duration(endTick) * 每个间隔经过的时间(fillInterval)
	endTime := tb.startTime.Add(time.Duration(endTick) * tb.fillInterval)
	// 等待的时间 = waitTime = endTime - take传入参数的开始时间(now)
	waitTime := endTime.Sub(now)
	// 等待超时
	if waitTime > maxWait {
		return 0, false //表明过了 0 ns 立即失败，不能取走
	}
	tb.availableTokens = avail
	return waitTime, true //表明过了 waitTime 成功，能取走
}

// currentTick 返回当前进过的时间间隔数，测量从 startTime 到现在过了几个间隔
func (tb *Bucket) currentTick(now time.Time) int64 {
	return int64(now.Sub(tb.startTime) / tb.fillInterval) // 经过时间 / 时间间隔
}

// adjustavailableTokens 调整当前令牌的数量
//tick - tb.latestTick 必须 > 0，使得在给定的时间，使得令牌是可用的，
func (tb *Bucket) adjustavailableTokens(tick int64) {
	if tb.availableTokens >= tb.capacity { // 可用令牌数 >= 总量
		return
	}
	//当前令牌数 = 上一次剩余的令牌数 + 距离上次放置令牌的时间间隔数 * 每次放置的令牌数
	tb.availableTokens += (tick - tb.latestTick) * tb.quantum
	if tb.availableTokens > tb.capacity { //如果 剩余令牌数 > 总量 (满了溢出)，就要 令其相等
		tb.availableTokens = tb.capacity
	}
	tb.latestTick = tick //更新最新令牌数
	return
}

// Clock 以一种方式表示时间的流逝
//可以被伪造出来用于测试。
type Clock interface {
	// Now returns the current time.
	Now() time.Time
	// Sleep sleeps for at least the given duration.
	Sleep(d time.Duration)
}

// realClock 意为真的时钟，实现时钟的标准时间函数。
type realClock struct{}

// Now 通过调用 time.Now 函数实现 Clock 接口.
func (realClock) Now() time.Time {
	return time.Now()
}

// Sleep 通过调用 time.Sleep 函数实现 Clock 接口.
func (realClock) Sleep(d time.Duration) {
	time.Sleep(d)
}
