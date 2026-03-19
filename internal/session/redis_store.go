package session

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const redisKeyPrefix = "session:"

// RedisStore implements Store backed by Redis. It provides shared session storage
// across multiple core instances for horizontal scaling.
type RedisStore struct {
	client *redis.Client
}

// NewRedisStore creates a Redis-backed session store.
func NewRedisStore(client *redis.Client) *RedisStore {
	return &RedisStore{client: client}
}

func (rs *RedisStore) key(id string) string {
	return redisKeyPrefix + id
}

func (rs *RedisStore) userKey(userID string) string {
	return "user_sessions:" + userID
}

func (rs *RedisStore) Create(ctx context.Context, s *Session) error {
	data, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("marshaling session: %w", err)
	}

	ttl := time.Until(s.ExpiresAt)
	if ttl <= 0 {
		return fmt.Errorf("session already expired")
	}

	pipe := rs.client.Pipeline()
	pipe.Set(ctx, rs.key(s.ID), data, ttl)
	pipe.SAdd(ctx, rs.userKey(s.UserID), s.ID)
	pipe.Expire(ctx, rs.userKey(s.UserID), 24*time.Hour)
	_, err = pipe.Exec(ctx)
	return err
}

func (rs *RedisStore) Get(ctx context.Context, id string) (*Session, error) {
	data, err := rs.client.Get(ctx, rs.key(id)).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("session %s not found", id)
		}
		return nil, fmt.Errorf("getting session: %w", err)
	}

	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("unmarshaling session: %w", err)
	}

	if s.isExpired() {
		rs.client.Del(ctx, rs.key(id))
		return nil, fmt.Errorf("session %s expired", id)
	}

	return &s, nil
}

func (rs *RedisStore) Delete(ctx context.Context, id string) error {
	// Try to get the session to clean up the user index.
	s, _ := rs.Get(ctx, id)
	if s != nil {
		rs.client.SRem(ctx, rs.userKey(s.UserID), id)
	}
	return rs.client.Del(ctx, rs.key(id)).Err()
}

func (rs *RedisStore) DeleteByUser(ctx context.Context, userID string) error {
	members, err := rs.client.SMembers(ctx, rs.userKey(userID)).Result()
	if err != nil && err != redis.Nil {
		return fmt.Errorf("listing user sessions: %w", err)
	}

	if len(members) > 0 {
		keys := make([]string, len(members))
		for i, id := range members {
			keys[i] = rs.key(id)
		}
		rs.client.Del(ctx, keys...)
	}

	return rs.client.Del(ctx, rs.userKey(userID)).Err()
}

func (rs *RedisStore) List(ctx context.Context, userID string) ([]*Session, error) {
	members, err := rs.client.SMembers(ctx, rs.userKey(userID)).Result()
	if err != nil {
		if err == redis.Nil {
			return []*Session{}, nil
		}
		return nil, fmt.Errorf("listing user sessions: %w", err)
	}

	var sessions []*Session
	for _, id := range members {
		s, err := rs.Get(ctx, id)
		if err != nil {
			// Session expired or missing — clean up the index.
			rs.client.SRem(ctx, rs.userKey(userID), id)
			continue
		}
		sessions = append(sessions, s)
	}

	if sessions == nil {
		sessions = []*Session{}
	}
	return sessions, nil
}

func (rs *RedisStore) Touch(ctx context.Context, id string, ttl time.Duration) error {
	s, err := rs.Get(ctx, id)
	if err != nil {
		return err
	}

	s.ExpiresAt = time.Now().Add(ttl)
	data, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("marshaling session: %w", err)
	}

	return rs.client.Set(ctx, rs.key(id), data, ttl).Err()
}
