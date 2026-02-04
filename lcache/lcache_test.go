package lcache_test

import (
	"testing"
	"time"

	"github.com/gookit/ext/lcache"
	"github.com/gookit/goutil/testutil/assert"
)

func TestSetAndGet(t *testing.T) {
	t.Run("set and get value with TTL", func(t *testing.T) {
		key := "testKey"
		value := "testValue"
		ttl := 1 * time.Second

		lcache.Set(key, value, ttl)
		result, found := lcache.Get[string](key)

		assert.True(t, found)
		assert.Equal(t, value, result)

		// Wait for TTL to expire
		time.Sleep(ttl + 100*time.Millisecond)
		_, found = lcache.Get[string](key)
		assert.False(t, found)
	})

	t.Run("get non-existent key", func(t *testing.T) {
		key := "nonExistentKey"
		var zero string

		result, found := lcache.Get[string](key)

		assert.False(t, found)
		assert.Equal(t, zero, result)
	})
}

func TestMGetAndMSet(t *testing.T) {
	keys := []string{"key1", "key2"}
	lcache.MSet(map[string]any{"key1": "value1", "key2": "value2"}, 1*time.Second)

	result := lcache.MGet(keys...)
	assert.ContainsKeys(t, result, keys)

	assert.Equal(t, "value1", lcache.Val("key1"))
	assert.Equal(t, "value2", lcache.Val("key2"))

	val, ok := lcache.Any("key1")
	assert.True(t, ok)
	assert.Equal(t, "value1", val)
}

func TestKeys(t *testing.T) {
	lcache.Clear()
	lcache.Set("key1", "value1", 1*time.Second)
	lcache.Set("key2", "value2", 1*time.Second)

	keys := lcache.Keys()

	assert.Contains(t, keys, "key1")
	assert.Contains(t, keys, "key2")
	assert.Len(t, keys, 2)
}

func TestLen(t *testing.T) {
	lcache.Clear() // Ensure clean state
	lcache.Set("key1", "value1", 1*time.Second)
	lcache.Set("key2", "value2", 1*time.Second)

	length := lcache.Len()
	assert.Equal(t, 2, length)
}

func TestClear(t *testing.T) {
	lcache.Set("key1", "value1", 1*time.Second)
	lcache.Set("key2", "value2", 1*time.Second)
	assert.Eq(t, "value1", lcache.Val("key1"))

	lcache.Clear()
	length := lcache.Len()
	assert.Equal(t, 0, length)
}

func TestDelete(t *testing.T) {
	lcache.Clear()
	key := "keyToDelete"
	lcache.Set(key, "value", 1*time.Second)

	lcache.Delete(key)
	_, found := lcache.Get[string](key)

	assert.False(t, found)
}

func TestOptions(t *testing.T) {
	assert.Panics(t, func() {
		lcache.Configure(lcache.WithSerializer("json2"))
	})

	lcache.SetSerializer("json2", lcache.JSONSerializer{})
	assert.NotPanics(t, func() {
		lcache.Configure(lcache.WithSerializer("json2"))
	})

	// delete serializer
	lcache.SetSerializer("json2", nil)
	assert.Panics(t, func() {
		lcache.Configure(lcache.WithSerializer("json2"))
	})
	lcache.Reset()
}

func TestSaveFileAndLoadFile(t *testing.T) {
	lcache.Configure(lcache.WithCapacity(50))
	lcache.Clear()
	lcache.Set("key1", "value1", 10*time.Second)
	lcache.Set("key2", "value2", 10*time.Second)

	filename := "testdata/test_cache.json"
	err := lcache.SaveFile(filename)
	assert.NoError(t, err)

	lcache.Clear()
	assert.Empty(t, lcache.Keys())

	err = lcache.LoadFile(filename)
	assert.NoError(t, err)

	val1, found1 := lcache.Get[string]("key1")
	assert.True(t, found1)
	assert.Equal(t, "value1", val1)
	val2, found2 := lcache.Get[string]("key2")
	assert.True(t, found2)
	assert.Equal(t, "value2", val2)
	lcache.Clear()
}
