package ratelimit

import (
	"sync"
	"time"

	"github.com/gofaquan/uber-go-ratelimit/internal/clock"
)

// Note: This file is inspired by:
//"go.uber.org/ratelimit/internal/clock"

// Limiter is used to rate-limit some process, possibly across goroutines.
// The process is expected to call Take() before every iteration, which
// may block to throttle the goroutine.
// Limiter 限制器用于限速某些进程，可能跨越 goroutines。
//进程在每次迭代之前调用 Take()
//可能会阻塞，以节流 goroutine。
type Limiter interface {
	// Take should block to make sure that the RPS is met.
	// Take 方法应该阻塞已确保满足 RPS (revolutions per second)
	Take() time.Time
}

// Clock is the minimum necessary interface to instantiate a rate limiter with
// a clock or mock clock, compatible with clocks created using
// github.com/andres-erbsen/clock.
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

// Take blocks to ensure that the time spent between multiple
// Take calls is on average time.Second/rate.
func (t *limiter) Take() time.Time {
	t.Lock()
	defer t.Unlock()

	now := t.clock.Now()

	// If this is our first request, then we allow it.
	if t.last.IsZero() {
		t.last = now
		return t.last
	}

	// sleepFor calculates how much time we should sleep based on
	// the perRequest budget and how long the last request took.
	// Since the request may take longer than the budget, this number
	// can get negative, and is summed across requests.
	t.sleepFor += t.perRequest - now.Sub(t.last)

	// We shouldn't allow sleepFor to get too negative, since it would mean that
	// a service that slowed down a lot for a short period of time would get
	// a much higher RPS following that.
	if t.sleepFor < t.maxSlack {
		t.sleepFor = t.maxSlack
	}

	// If sleepFor is positive, then we should sleep now.
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

// NewUnlimited returns a RateLimiter that is not limited.
func NewUnlimited() Limiter {
	return unlimited{}
}

func (unlimited) Take() time.Time {
	return time.Now()
}
