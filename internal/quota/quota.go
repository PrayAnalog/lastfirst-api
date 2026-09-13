package quota

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

const readReserve = 500

var ErrExhausted = errors.New("YouTube API quota exhausted")

var pacific = func() *time.Location {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		panic(err)
	}
	return loc
}()

type Counter struct {
	rdb   *redis.Client
	limit int64
}

func New(rdb *redis.Client, limit int64) *Counter {
	return &Counter{rdb: rdb, limit: limit}
}

func key(now time.Time) string {
	return "quota:" + now.In(pacific).Format(time.DateOnly)
}

func (c *Counter) incr(ctx context.Context, n int64) (int64, error) {
	k := key(time.Now())
	pipe := c.rdb.TxPipeline()
	v := pipe.IncrBy(ctx, k, n)
	pipe.Expire(ctx, k, 48*time.Hour)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, err
	}
	return v.Val(), nil
}

func (c *Counter) Add(ctx context.Context, n int64) error {
	_, err := c.incr(ctx, n)
	return err
}

func (c *Counter) Reserve(ctx context.Context, n int64) error {
	v, err := c.incr(ctx, n)
	if err != nil {
		return err
	}
	if v > c.limit-readReserve {
		c.rdb.DecrBy(ctx, key(time.Now()), n)
		return ErrExhausted
	}
	return nil
}

func (c *Counter) Exhaust(ctx context.Context) error {
	return c.rdb.Set(ctx, key(time.Now()), c.limit, 48*time.Hour).Err()
}

func (c *Counter) Used(ctx context.Context) (int64, error) {
	v, err := c.rdb.Get(ctx, key(time.Now())).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	return v, err
}

func (c *Counter) Limit() int64 {
	return c.limit
}

func NextReset(now time.Time) time.Time {
	t := now.In(pacific)
	return time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, pacific)
}
