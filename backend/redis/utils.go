package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/golang/snappy"
	"github.com/highlight-run/highlight/backend/hlog"
	"github.com/highlight-run/highlight/backend/model"
	"github.com/highlight-run/highlight/backend/util"
	"github.com/openlyinc/pointy"
	"github.com/pkg/errors"
)

type Client struct {
	redisClient redis.Cmdable
}

var (
	ServerAddr = os.Getenv("REDIS_EVENTS_STAGING_ENDPOINT")
)

func UseRedis(projectId int, sessionSecureId string) bool {
	return true
}

func EventsKey(sessionId int) string {
	return fmt.Sprintf("events-%d", sessionId)
}

func SessionInitializedKey(sessionSecureId string) string {
	return fmt.Sprintf("session-init-%s", sessionSecureId)
}

func NewClient() *Client {
	if util.IsDevOrTestEnv() {
		return &Client{
			redisClient: redis.NewClient(&redis.Options{
				Addr:         ServerAddr,
				Password:     "",
				ReadTimeout:  5 * time.Second,
				WriteTimeout: 5 * time.Second,
				MaxConnAge:   5 * time.Minute,
				IdleTimeout:  5 * time.Minute,
				MaxRetries:   5,
				MinIdleConns: 16,
				PoolSize:     256,
			}),
		}
	} else {
		c := redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:        []string{ServerAddr},
			Password:     "",
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 5 * time.Second,
			MaxConnAge:   5 * time.Minute,
			IdleTimeout:  5 * time.Minute,
			MaxRetries:   5,
			MinIdleConns: 16,
			PoolSize:     256,
			OnConnect: func(context.Context, *redis.Conn) error {
				hlog.Incr("redis.new-conn", nil, 1)
				return nil
			},
		})
		go func() {
			for {
				stats := c.PoolStats()
				if stats == nil {
					return
				}
				hlog.Histogram("redis.hits", float64(stats.Hits), nil, 1)
				hlog.Histogram("redis.misses", float64(stats.Misses), nil, 1)
				hlog.Histogram("redis.idle-conns", float64(stats.IdleConns), nil, 1)
				hlog.Histogram("redis.stale-conns", float64(stats.StaleConns), nil, 1)
				hlog.Histogram("redis.total-conns", float64(stats.TotalConns), nil, 1)
				hlog.Histogram("redis.timeouts", float64(stats.Timeouts), nil, 1)
				time.Sleep(time.Second)
			}
		}()
		return &Client{
			redisClient: c,
		}
	}
}

func (r *Client) RemoveValues(ctx context.Context, sessionId int, valuesToRemove []interface{}) error {
	cmd := r.redisClient.ZRem(ctx, EventsKey(sessionId), valuesToRemove...)
	if cmd.Err() != nil {
		return errors.Wrap(cmd.Err(), "error removing values from Redis")
	}
	return nil
}

func (r *Client) GetRawZRange(ctx context.Context, sessionId int, nextPayloadId int) ([]redis.Z, error) {
	maxScore := "(" + strconv.FormatInt(int64(nextPayloadId), 10)

	vals, err := r.redisClient.ZRangeByScoreWithScores(ctx, EventsKey(sessionId), &redis.ZRangeBy{
		Min: "-inf",
		Max: maxScore,
	}).Result()
	if err != nil {
		return nil, errors.Wrap(err, "error retrieving prior events from Redis")
	}

	return vals, nil
}

func (r *Client) GetEventObjects(ctx context.Context, s *model.Session, cursor model.EventsCursor, events map[int]string) ([]model.EventsObject, error, *model.EventsCursor, bool) {
	// Session is live if the cursor is not the default
	isLive := cursor != model.EventsCursor{}

	eventObjectIndex := "-inf"
	if cursor.EventObjectIndex != nil {
		eventObjectIndex = "(" + strconv.FormatInt(int64(*cursor.EventObjectIndex), 10)
	}

	vals, err := r.redisClient.ZRangeByScoreWithScores(ctx, EventsKey(s.ID), &redis.ZRangeBy{
		Min: eventObjectIndex,
		Max: "+inf",
	}).Result()
	if err != nil {
		return nil, errors.Wrap(err, "error retrieving events from Redis"), nil, false
	}

	endsWithBeacon := false
	for idx, z := range vals {
		intScore := int(z.Score)
		if idx != len(vals)-1 {
			endsWithBeacon = z.Score != float64(intScore)
		}
		// Beacon events have decimals, skip them if it's live mode or not the last event
		if z.Score != float64(intScore) && (isLive || idx != len(vals)-1) {
			continue
		}

		events[intScore] = z.Member.(string)
	}

	keys := make([]int, 0, len(events))
	for k := range events {
		keys = append(keys, k)
	}
	sort.Ints(keys)

	eventsObjects := []model.EventsObject{}
	if len(keys) == 0 {
		return eventsObjects, nil, &cursor, endsWithBeacon
	}

	maxScore := keys[len(keys)-1]

	for _, k := range keys {
		asBytes := []byte(events[k])

		// Messages may be encoded with `snappy`.
		// Try decoding them, but if decoding fails, use the original message.
		decoded, err := snappy.Decode(nil, asBytes)
		if err != nil {
			decoded = asBytes
		}

		eventsObjects = append(eventsObjects, model.EventsObject{
			Events: string(decoded),
		})
	}

	nextCursor := model.EventsCursor{EventIndex: 0, EventObjectIndex: pointy.Int(maxScore)}
	return eventsObjects, nil, &nextCursor, endsWithBeacon
}

func (r *Client) GetEvents(ctx context.Context, s *model.Session, cursor model.EventsCursor, events map[int]string) ([]interface{}, error, *model.EventsCursor, bool) {
	allEvents := make([]interface{}, 0)

	eventsObjects, err, newCursor, endsWithBeacon := r.GetEventObjects(ctx, s, cursor, events)
	if err != nil {
		return nil, errors.Wrap(err, "error getting events objects"), nil, false
	}

	if len(eventsObjects) == 0 {
		return allEvents, nil, &cursor, false
	}

	for _, obj := range eventsObjects {
		subEvents := make(map[string][]interface{})
		if err := json.Unmarshal([]byte(obj.Events), &subEvents); err != nil {
			return nil, errors.Wrap(err, "error decoding event data"), nil, false
		}
		allEvents = append(allEvents, subEvents["events"]...)
	}

	if cursor.EventIndex != 0 && cursor.EventIndex < len(allEvents) {
		allEvents = allEvents[cursor.EventIndex:]
	}

	return allEvents, nil, newCursor, endsWithBeacon
}

func (r *Client) AddEventPayload(ctx context.Context, sessionID int, score float64, payload string) error {
	encoded := string(snappy.Encode(nil, []byte(payload)))

	// Calls ZADD, and if the key does not exist yet, sets an expiry of 4h10m.
	var zAddAndExpire = redis.NewScript(`
		local key = KEYS[1]
		local score = ARGV[1]
		local value = ARGV[2]

		local count = redis.call("EXISTS", key)
		redis.call("ZADD", key, score, value)

		if count == 0 then
			redis.call("EXPIRE", key, 30000)
		end

		return
	`)

	keys := []string{EventsKey(sessionID)}
	values := []interface{}{score, encoded}
	cmd := zAddAndExpire.Run(ctx, r.redisClient, keys, values...)

	if err := cmd.Err(); err != nil && !errors.Is(err, redis.Nil) {
		return errors.Wrap(err, "error adding events payload in Redis")
	}
	return nil
}

func (r *Client) setFlag(ctx context.Context, key string, value bool, exp time.Duration) error {
	cmd := r.redisClient.Set(ctx, key, value, exp)
	if cmd.Err() != nil {
		return errors.Wrap(cmd.Err(), "error setting flag from Redis")
	}
	return nil
}

func (r *Client) IsPendingSession(ctx context.Context, sessionSecureId string) (bool, error) {
	key := SessionInitializedKey(sessionSecureId)
	val, err := r.redisClient.Get(ctx, key).Result()

	// ignore the non-existing session keys
	if err == redis.Nil {
		return false, nil
	} else if err != nil {
		return false, errors.Wrap(err, "error getting flag from Redis")
	}
	return val == "1" || val == "true", nil
}

func (r *Client) SetIsPendingSession(ctx context.Context, sessionSecureId string, initialized bool) error {
	return r.setFlag(ctx, SessionInitializedKey(sessionSecureId), initialized, 24*time.Hour)
}
