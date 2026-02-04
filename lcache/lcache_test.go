package lcache_test

import (
	"testing"
	"time"

	"github.com/gookit/goutil/testutil/assert"
	"github.com/gookit/goutil/x/lcache"
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

func TestSaveFileAndLoadFile(t *testing.T) {
	t.Run("json", func(t *testing.T) {
		lcache.Configure(lcache.WithCapacity(50))
		filename := "testdata/test_cache.json"
		saveFileAndLoadFile(filename, t)
	})

	// t.Run("gob", func(t *testing.T) {
	// 	lcache.Configure(lcache.WithSerializer("gob"))
	// 	filename := "testdata/test_cache.gob"
	// 	saveFileAndLoadFile(filename, t)
	// 	lcache.Configure(lcache.WithSerializer("json"))
	// })
}

func saveFileAndLoadFile(filename string, t *testing.T) {
	lcache.Clear()

	lcache.Set("key1", "value1", 10*time.Second)
	lcache.Set("key2", "value2", 10*time.Second)
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
