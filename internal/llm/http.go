package llm

import (
	"math/rand"
	"time"
)

const (
	defaultRetry     int           = 5
	defaultBaseDelay time.Duration = time.Millisecond * 100
	defaultMaxDelay  time.Duration = 10 * time.Second
)

// backoff 计算第 attempt 次（从 0 开始）重试的等待时间。
// 策略：指数退避 base * 2^attempt，再叠加随机抖动(jitter)。
// 抖动避免多个客户端在同一时刻集中重试（"惊群"问题）。
func backoff(attempt int) time.Duration {
	delay := defaultBaseDelay * (1 << attempt)
	if delay > defaultMaxDelay {
		delay = defaultMaxDelay
	}
	jitter := time.Duration(rand.Int63n(int64(delay)))
	return delay + jitter
}

func isRetryable(code int) bool {
	if code == 400 || code == 401 {
		return false
	}
	return true
}
