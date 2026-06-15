package redis

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const (
	lockKey  = "keygen:refill:lock"
	luaCheck = `
local len = redis.call("LLEN", KEYS[1])
if len < tonumber(ARGV[1]) then
    return redis.call("SET", KEYS[2], ARGV[2], "NX", "EX", ARGV[3])
end
return nil
`
)

type RefillLock struct {
	client     *goredis.Client
	instanceID string
	ttl        time.Duration
	script     *goredis.Script
}

func NewRefillLock(client *goredis.Client, instanceID string, ttl time.Duration) *RefillLock {
	return &RefillLock{
		client:     client,
		instanceID: instanceID,
		ttl:        ttl,
		script:     goredis.NewScript(luaCheck),
	}
}

func (l *RefillLock) AcquireIfBelow(ctx context.Context, threshold int64) (bool, error) {
	result, err := l.script.Run(ctx, l.client,
		[]string{queueKey, lockKey},
		threshold, l.instanceID, int(l.ttl.Seconds()),
	).Result()
	if err != nil {
		if err == goredis.Nil {
			return false, nil
		}
		return false, fmt.Errorf("refill lock lua: %w", err)
	}
	return result == "OK", nil
}
