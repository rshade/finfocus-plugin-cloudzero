package cache

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCacheGet(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		value  any
		ttl    time.Duration
		lookup string
		wantOK bool
	}{
		{name: "hit", value: "value", ttl: time.Hour, lookup: "key", wantOK: true},
		{name: "miss", value: "value", ttl: time.Hour, lookup: "other", wantOK: false},
		{name: "expired", value: "value", ttl: -time.Second, lookup: "key", wantOK: false},
		{name: "int", value: 42, ttl: time.Hour, lookup: "key", wantOK: true},
		{name: "map", value: map[string]string{"a": "b"}, ttl: time.Hour, lookup: "key", wantOK: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cache := NewCache()
			cache.Set("key", tt.value, tt.ttl)
			got, ok := cache.Get(tt.lookup)
			assert.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				assert.Equal(t, tt.value, got)
			} else {
				assert.Nil(t, got)
			}
		})
	}
}

func TestCacheConcurrentAccess(t *testing.T) {
	t.Parallel()
	cache := NewCache()
	var wg sync.WaitGroup
	wg.Go(func() {
		for range 100 {
			cache.Set("key", "value", time.Hour)
		}
	})
	wg.Go(func() {
		for range 100 {
			cache.Get("key")
		}
	})
	wg.Wait()
}
