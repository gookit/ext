package lcache_test

import (
	"testing"
	"time"

	"github.com/gookit/ext/lcache"
	"github.com/gookit/goutil/testutil/assert"
)

func TestCache_SetAndGet(t *testing.T) {
	c := lcache.New()

	t.Run("basic", func(t *testing.T) {
		// Test basic set and get
		c.Set("key1", "value1", 5*time.Minute)
		val, found := c.Get("key1")
		assert.True(t, found)
		assert.Eq(t, "value1", val)

		// Test non-existent key
		_, found = c.Get("non-existent")
		assert.False(t, found)
		c.Clear()

		assert.Empty(t, c.Keys())
		assert.Eq(t, c.Len(), 0)
	})

	t.Run("str", func(t *testing.T) {
		c.Set("str", "hello", 5*time.Minute)
		val, found := c.Get("str")
		assert.True(t, found)
		assert.Eq(t, "hello", val)

		// Test mismatch
		_, found = c.Get("int")
		assert.False(t, found)
		c.Clear()
	})

	t.Run("int", func(t *testing.T) {
		c.Set("num", 42, 5*time.Minute)
		val, found := c.Get("num")
		assert.True(t, found)
		assert.Eq(t, 42, val)

		// Test mismatch
		_, found = c.Get("str")
		assert.False(t, found)
		c.Clear()
	})

	t.Run("bool", func(t *testing.T) {
		c.Set("flag", true, 5*time.Minute)
		val, found := c.Get("flag")
		assert.True(t, found)
		assert.True(t, val.(bool))

		// Test type mismatch
		_, found = c.Get("str")
		assert.False(t, found)
		c.Clear()
	})
}

func TestCache_Expiration(t *testing.T) {
	c := lcache.New()
	defer c.Clear()

	// Set with short TTL
	c.Set("short", "Val", 100*time.Millisecond)
	val, found := c.Get("short")
	assert.True(t, found)
	assert.Eq(t, "Val", val)

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)
	_, found = c.Get("short")
	assert.False(t, found)
}

func TestCache_NoExpiration(t *testing.T) {
	c := lcache.New()
	defer c.Clear()

	// Set with duration <= 0 (never expires)
	c.Set("permanent", "Val", 0)
	val, found := c.Get("permanent")
	assert.True(t, found)
	assert.Eq(t, "Val", val)

	// Wait a bit and check again
	time.Sleep(100 * time.Millisecond)
	val, found = c.Get("permanent")
	assert.True(t, found)
	assert.Eq(t, "Val", val)
}
func TestCache_MGet(t *testing.T) {
	c := lcache.New()
	c.Set("k1", "val1", time.Second*10)
	c.Set("k2", "val2", time.Second*10)

	t.Run("all keys exist", func(t *testing.T) {
		result := c.MGet("k1", "k2")
		assert.Equal(t, "val1", result["k1"])
		assert.Equal(t, "val2", result["k2"])
	})

	t.Run("some keys missing", func(t *testing.T) {
		result := c.MGet("k1", "missing")
		assert.Equal(t, "val1", result["k1"])
		assert.Nil(t, result["missing"])
	})
}

func TestCache_MSet(t *testing.T) {
	c := lcache.New()

	t.Run("set multiple items", func(t *testing.T) {
		items := map[string]any{
			"k1": "val1",
			"k2": "val2",
		}
		c.MSet(items, time.Second*10)

		val1, ok := c.Get("k1")
		assert.True(t, ok)
		assert.Equal(t, "val1", val1)

		val2, ok := c.Get("k2")
		assert.True(t, ok)
		assert.Equal(t, "val2", val2)
	})

	t.Run("update existing items", func(t *testing.T) {
		c.Set("k1", "old_val", time.Second*10)
		items := map[string]any{
			"k1": "new_val",
		}
		c.MSet(items, time.Second*10)

		val, ok := c.Get("k1")
		assert.True(t, ok)
		assert.Equal(t, "new_val", val)
	})
}

func TestCache_Delete(t *testing.T) {
	c := lcache.New()
	defer c.Clear()

	c.Set("key", "Val", 5*time.Minute)
	_, found := c.Get("key")
	assert.True(t, found)

	c.Delete("key")
	_, found = c.Get("key")
	assert.False(t, found)

	// Delete non-existent key should not panic
	has := c.Delete("non-existent")
	assert.False(t, has)
}

func TestCache_Has(t *testing.T) {
	c := lcache.New()
	defer c.Clear()

	c.Set("key", "Val", 5*time.Minute)
	assert.True(t, c.Has("key"))
	assert.False(t, c.Has("non-existent"))
}

func TestCache_Keys(t *testing.T) {
	c := lcache.New()
	defer c.Clear()

	c.Set("key1", "value1", 5*time.Minute)
	c.Set("key2", "value2", 5*time.Minute)
	c.Set("key3", "value3", 5*time.Minute)

	keys := c.Keys()
	assert.Eq(t, 3, len(keys))
	assert.Contains(t, keys, "key1")
	assert.Contains(t, keys, "key2")
	assert.Contains(t, keys, "key3")
}

func TestCache_Len(t *testing.T) {
	c := lcache.New()
	defer c.Clear()

	assert.Eq(t, 0, c.Len())

	c.Set("key1", "value1", 5*time.Minute)
	assert.Eq(t, 1, c.Len())

	c.Set("key2", "value2", 5*time.Minute)
	assert.Eq(t, 2, c.Len())
}

func TestCache_Clear(t *testing.T) {
	c := lcache.New()
	defer c.Clear()

	c.Set("key1", "value1", 5*time.Minute)
	c.Set("key2", "value2", 5*time.Minute)
	assert.Eq(t, 2, c.Len())

	c.Clear()
	assert.Eq(t, 0, c.Len())
	assert.False(t, c.Has("key1"))
	assert.False(t, c.Has("key2"))
}

func TestCache_Concurrent(t *testing.T) {
	c := lcache.New()
	defer c.Clear()

	// Concurrent writes
	for i := 0; i < 100; i++ {
		go func(n int) {
			c.Set("key", n, 5*time.Minute)
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 100; i++ {
		go func() {
			c.Get("key")
		}()
	}

	// Wait a bit for goroutines to complete
	time.Sleep(100 * time.Millisecond)

	// Cache should still be in a valid state
	_, found := c.Get("key")
	assert.True(t, found)
}
