package main

import (
	"context"
	"strconv"
	"sync"
	"time"
)

// entry is one value in the store, plus its optional expiry time
type entry struct {
	value   string
	expires time.Time
	hasTTL  bool
}

// Store is our in-memory key-value database. All access goes through
// the mutex so multiple client goroutines can use it at the same time.
type Store struct {
	mu   sync.RWMutex
	data map[string]entry
}

func NewStore() *Store {
	return &Store{
		data: make(map[string]entry),
	}
}

func (s *Store) Set(key string, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = entry{value: value}
}

func (s *Store) SetWithTTL(key string, value string, seconds int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = entry{
		value:   value,
		expires: time.Now().Add(time.Duration(seconds) * time.Second),
		hasTTL:  true,
	}
}

func (s *Store) Get(key string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.data[key]
	if !ok {
		return "", false
	}

	if e.hasTTL && time.Now().After(e.expires) {
		delete(s.data, key)
		return "", false
	}

	return e.value, true
}

func (s *Store) Del(keys []string) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	count := 0
	for _, key := range keys {
		if _, ok := s.data[key]; ok {
			delete(s.data, key)
			count++
		}
	}
	return count
}

func (s *Store) Exists(keys []string) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	count := 0
	for _, key := range keys {
		e, ok := s.data[key]
		if !ok {
			continue
		}
		if e.hasTTL && time.Now().After(e.expires) {
			delete(s.data, key)
			continue
		}
		count++
	}
	return count
}

func (s *Store) Keys() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	keys := make([]string, 0, len(s.data))
	for k, e := range s.data {
		if e.hasTTL && now.After(e.expires) {
			delete(s.data, k)
			continue
		}
		keys = append(keys, k)
	}
	return keys
}

// TTL returns seconds left, -1 if the key has no expiry, -2 if it doesn't exist
func (s *Store) TTL(key string) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.data[key]
	if !ok {
		return -2
	}

	if !e.hasTTL {
		return -1
	}

	if time.Now().After(e.expires) {
		delete(s.data, key)
		return -2
	}

	left := time.Until(e.expires).Seconds()
	if left < 0 {
		left = 0
	}
	return int(left) + 1
}

// Expire sets a TTL on an existing key, returns 1 if it worked, 0 if the key is missing
func (s *Store) Expire(key string, seconds int) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.data[key]
	if !ok {
		return 0
	}
	if e.hasTTL && time.Now().After(e.expires) {
		delete(s.data, key)
		return 0
	}

	e.hasTTL = true
	e.expires = time.Now().Add(time.Duration(seconds) * time.Second)
	s.data[key] = e
	return 1
}

// Incr adds 1 to the number stored at key, treats missing key as 0
func (s *Store) Incr(key string) (int, error) {
	return s.addTo(key, 1)
}

func (s *Store) Decr(key string) (int, error) {
	return s.addTo(key, -1)
}

func (s *Store) addTo(key string, amount int) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.data[key]
	if ok && e.hasTTL && time.Now().After(e.expires) {
		ok = false
	}

	current := 0
	if ok {
		n, err := strconv.Atoi(e.value)
		if err != nil {
			return 0, err
		}
		current = n
	}

	current += amount
	newValue := entry{value: strconv.Itoa(current)}
	if ok && e.hasTTL {
		newValue.hasTTL = true
		newValue.expires = e.expires
	}
	s.data[key] = newValue

	return current, nil
}

// sweepExpired walks the whole map and removes anything that has expired.
// This is the "active" expiry, it runs on a timer in the background so
// keys that nobody ever reads still get cleaned up eventually.
func (s *Store) sweepExpired() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for k, e := range s.data {
		if e.hasTTL && now.After(e.expires) {
			delete(s.data, k)
		}
	}
}

// StartSweeper runs sweepExpired on a timer until ctx is cancelled.
func (s *Store) StartSweeper(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				s.sweepExpired()
			case <-ctx.Done():
				return
			}
		}
	}()
}
