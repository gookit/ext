// Package lcache provides a simple, goroutine-safe local cache implementation with TTL support.
//
// Quickly usage:
//
//	lcache.Set("key", "value", 5*time.Minute)
//
//	val, found := lcache.Get("key")
//	if found {
//	    fmt.Println(val)
//	}
//
// Custom configuration:
//
//	cache := lcache.New(
//		lcache.WithCapacity(10),
//	)
package lcache

import (
	"encoding/json"
	"io"
	"time"

	"github.com/gookit/goutil/comdef"
	"github.com/gookit/goutil/strutil"
)

// std 默认的全局缓存实例
var std = New()

// Reset the default cache instance
func Reset() { std = New() }

// Configure the default cache settings
func Configure(optFns ...OptionFn) { std.Configure(optFns...) }

// Val get value by key
func Val(key string) any { return std.Val(key) }

// Any get any type value by key
func Any(key string) (val any, ok bool) { return std.Get(key) }

// Set value by key with TTL
func Set[T any](key string, val T, ttl time.Duration) {
	std.Set(key, val, ttl)
}

// Get typed value by key, return zero value if not found
func Get[T any](key string) (T, bool) {
	return TypedInCache[T](std, key)
}

// MGet get multiple key-value pairs from the cache.
func MGet(keys ...string) map[string]any { return std.MGet(keys...) }

// MSet set multiple key-value pairs in the cache.
func MSet(items map[string]any, ttl time.Duration) { std.MSet(items, ttl) }

// CacheNotExist 表示缓存不存在的特殊值，避免缓存穿透
var CacheNotExist = "_cache_not_exist_"

// MGetOrFind 根据key prefix + keys(eg: ids) 批量获取缓存值，不存在则调用回调函数获取(eg: DB)数据
//
// 示例：
//
//	ids := []uint{1, 2, 3}
//	userList := cache.MGetElse("user:", ids, cacheTTL, func(keys []uint) map[uint]*models.User {
//			return db.ListByIDs(keys)
//	})
func MGetOrFind[K comdef.SimpleType, T any](keyPrefix string, keys []K, cacheTTL time.Duration, queryFn func(keys []K) map[K]T) []T {
	return MGetCacheOrFind(std, keyPrefix, keys, cacheTTL, queryFn)
}

// Keys get the keys of the default cache
func Keys() []string { return std.Keys() }

// Len get the number of items in the cache
func Len() int { return std.Len() }

// Clear all items from the default cache
func Clear() { std.Clear() }

// Delete key
func Delete(key string) { std.Delete(key) }

// MDelete delete multiple keys
func MDelete(keys ...string) { std.MDelete(keys...) }

// SaveFile Save the cache data to a file.
func SaveFile(filename string) error {
	return std.SaveFile(filename)
}

// LoadFile Recover cache data from file load
func LoadFile(filename string) error {
	return std.LoadFile(filename)
}

//
// ----- extend helpers -----
//

// Get typed value by key, return zero value if not found
func TypedInCache[T any](c *Cache, key string) (T, bool) {
	var zero T // 零值
	val, ok := c.Get(key)
	if !ok {
		return zero, false
	}

	// 类型断言
	res, ok := val.(T)
	if !ok {
		return zero, false
	}
	return res, true
}

// MGetCacheOrFind 根据key prefix + keys(eg: ids) 批量获取缓存值，不存在则调用回调函数获取(DB)数据
func MGetCacheOrFind[K comdef.SimpleType, T any](c *Cache, prefix string, keys []K, cacheTTL time.Duration, queryFn func(keys []K) map[K]T) []T {
	if len(keys) == 0 {
		return make([]T, 0)
	}

	// 构建完整的缓存键list
	fullKeys := make([]string, 0, len(keys))
	keyMap := make(map[string]K, len(keys))
	for _, key := range keys {
		keyStr := strutil.SafeString(key)
		fullKeys = append(fullKeys, prefix+keyStr)
		keyMap[keyStr] = key
	}

	dataList := make([]T, 0, len(keys))
	// 从缓存中获取值
	itemList := c.MGet(fullKeys...)

	var missKeys []K
	foundKeyMap := make(map[string]K)
	for fullKey, item := range itemList {
		keyStr := fullKey[len(prefix):]

		// 当缓存值为 CacheNotExist 时，跳过。但是记录为已找到
		if str, ok := item.(string); ok && str == CacheNotExist {
			foundKeyMap[keyStr] = keyMap[keyStr]
			continue
		}

		if val, ok := item.(T); ok {
			dataList = append(dataList, val)
			foundKeyMap[keyStr] = keyMap[keyStr]
		}
	}

	for _, key := range keys {
		if _, ok := foundKeyMap[strutil.SafeString(key)]; !ok {
			missKeys = append(missKeys, key)
		}
	}
	if len(missKeys) == 0 {
		return dataList
	}

	// 调用回调函数获取缺失的缓存值
	missDataMap := queryFn(missKeys)
	netSetMap := make(map[string]any, len(missKeys))

	// 合并结果
	for _, key := range missKeys {
		keyStr := strutil.SafeString(key)
		if val, ok := missDataMap[key]; ok {
			dataList = append(dataList, val)
			netSetMap[keyStr] = val
		} else {
			// 设置一个特殊值表示缓存不存在，避免缓存穿透
			netSetMap[keyStr] = CacheNotExist
		}
	}

	c.MSet(netSetMap, cacheTTL)
	return dataList
}

//
// ----- builtin serializers -----
//

type Serializer interface {
	comdef.Codec
	DecodeFrom(r io.Reader, dest any) error
	EncodeTo(w io.Writer, src any) error
}

var serializers = map[string]Serializer{
	"json": JSONSerializer{},
}

// SetSerializer set new serializer for the cache. if serializer is nil, delete it
func SetSerializer(name string, serializer Serializer) {
	if serializer != nil {
		serializers[name] = serializer
	} else {
		delete(serializers, name)
	}
}

// JSONSerializer builtin serializer: json, gob
type JSONSerializer struct{}

// Decode implements Serializer
func (j JSONSerializer) Decode(data []byte, dest any) error {
	return json.Unmarshal(data, dest)
}

// Encode implements Serializer
func (j JSONSerializer) Encode(data any) ([]byte, error) {
	return json.Marshal(data)
}

// DecodeFrom implements Serializer
func (j JSONSerializer) DecodeFrom(r io.Reader, dest any) error {
	return json.NewDecoder(r).Decode(dest)
}

// EncodeTo implements Serializer
func (j JSONSerializer) EncodeTo(w io.Writer, src any) error {
	return json.NewEncoder(w).Encode(src)
}

//
// ----- options for cache -----
//

// Options for cache
type Options struct {
	// Capacity maximum number of cached entries default is 1000
	Capacity int
	// Serializer name, use for save/load file.
	//
	// default is: "json". see JSONSerializer
	Serializer string
	// OnEvicted callback function on item evicted
	OnEvicted func(key string, value any)
}

// OptionFn option config func
type OptionFn func(*Options)

// WithCapacity set cache capacity
func WithCapacity(capacity int) OptionFn {
	return func(o *Options) {
		o.Capacity = capacity
	}
}

// WithSerializer specify serializer name. eg: "json", "gob"
func WithSerializer(serializer string) OptionFn {
	// check serializer name
	if _, ok := serializers[serializer]; !ok {
		panic("not registered serializer name: " + serializer)
	}

	return func(o *Options) {
		o.Serializer = serializer
	}
}

// WithOnEvictFn set cache item evicted callback function
func WithOnEvictFn(fn func(key string, value any)) OptionFn {
	return func(o *Options) {
		o.OnEvicted = fn
	}
}
