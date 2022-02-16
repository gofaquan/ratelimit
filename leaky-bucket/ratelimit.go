package leakyBucket

import (
	"github.com/gofaquan/leaky-bucket/internal/clock"
	"sync"
	"time"
)

// Note: This file is inspired by:
//"go.uber.org/ratelimit/"

// Limiter 限制器用于限速某些进程，可能跨越 goroutines。
//进程在每次迭代之前调用 Take()
//可能会阻塞，以节流 goroutine。
type Limiter interface {
	// Take 方法应该阻塞已确保满足 RPS (revolutions per second)
	Take() time.Time
}

// Clock 时钟是实例化 一个速率限制器 所需的 最小接口
//一个时钟或模拟时钟，兼容使用
type Clock interface {
	Now() time.Time
	Sleep(time.Duration)
}

type limiter struct {
	sync.Mutex               // 锁
	last       time.Time     // 上一次的时刻
	sleepFor   time.Duration // 需要等待的时间
	perRequest time.Duration // 每次的时间间隔
	maxSlack   time.Duration // 最大的富余量
	clock      Clock         // 时钟
}

// Option 用 Option设计模式 配置一个 Limiter 限制器.
type Option func(l *limiter)

// New 返回一个限制器，将限制给定的 RPS  (revolutions per second) 。
func New(rate int, opts ...Option) Limiter {
	l := &limiter{
		perRequest: time.Second / time.Duration(rate),       //每次的时间间隔 = 1 / rate 秒, eg: 1/3 = 333.333333 ms
		maxSlack:   -10 * time.Second / time.Duration(rate), // 最大的富余量 = -10 * rate 秒
	}
	//为上方的 limiter 配置 各种参数，如下方的 WithClock ，传入即可配置对应 clock 参数
	for _, opt := range opts {
		opt(l)
	}
	// 如果上方未配置 clock 参数，那就给他创建一个
	if l.clock == nil {
		l.clock = clock.New()
	}
	return l
}

// WithClock 返回一个 ratelimit.New 的 Option。
//提供替代方案的新时钟 Clock 的实现，通常是用于测试的模拟时钟。
func WithClock(clock Clock) Option {
	return func(l *limiter) {
		l.clock = clock
	}
}

// WithoutSlack 是 ratelimit.New 的一个初始化 Option。
// 初始化一个 没有任何初始容忍突发流量的 limiter 限制器。
var WithoutSlack Option = withoutSlackOption

func withoutSlackOption(l *limiter) {
	l.maxSlack = 0
}

//下面的代码根据记录每次请求的间隔时间和上一次请求的时刻来计算当次请求需要阻塞的时间 sleepFor ，
//这里需要留意的是 sleepFor 的值可能为负，在经过间隔时间长的两次访问之后会导致随后大量的请求被放行，
//所以代码中针对这个场景有专门的优化处理。创建限制器的 New() 函数中会为 maxSlack 设置初始值，
//也可以通过 WithoutSlack 这个 Option 取消这个默认值。

// Take 会阻塞确保两次请求之间的时间走完
// Take 调用平均数为 time.Second/rate.
func (t *limiter) Take() time.Time {
	t.Lock()
	defer t.Unlock()

	now := t.clock.Now()

	// 如果是第一次请求就直接放行
	if t.last.IsZero() {
		t.last = now
		return t.last
	}

	// sleepFor 根据 perRequest 和上一次请求的时刻计算应该 sleep 的时间
	// 由于每次请求间隔的时间可能会超过 perRequest, 所以这个数字可能为负数，并在多个请求之间累加
	t.sleepFor += t.perRequest - now.Sub(t.last)

	// 我们不应该让 sleepFor 负的太多，因为这意味着一个服务在短时间内慢了很多随后会得到更高的 RPS。
	if t.sleepFor < t.maxSlack {
		t.sleepFor = t.maxSlack
	}

	// 如果 sleepFor 是正值那么就 sleep
	if t.sleepFor > 0 {
		t.clock.Sleep(t.sleepFor)
		t.last = now.Add(t.sleepFor)
		t.sleepFor = 0
	} else {
		t.last = now
	}

	return t.last
}

type unlimited struct{}

// NewUnlimited 返回一个不受限制的 RateLimiter 限制器。
func NewUnlimited() Limiter {
	return unlimited{}
}

// Take 写一个 Take() 函数 实现 Limiter 限制器接口
func (unlimited) Take() time.Time {
	return time.Now()
}
