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
// 		lcache.WithCapacity(10),
//	)
package lcache

import (
	"encoding/json"
	"io"
	"time"

	"github.com/gookit/goutil/comdef"
)

// std 默认的全局缓存实例
var std = New()

// Reset the default cache instance
func Reset() { std = New() }

// Configure the default cache settings
func Configure(optFns ...OptionFn) { std.Configure(optFns...) }

// Set value by key with TTL
func Set[T any](key string, val T, ttl time.Duration) {
	std.Set(key, val, ttl)
}

// Val get value by key
func Val(key string) any { return std.Val(key) }

// Any get any type value by key
func Any(key string) (val any, ok bool) { return std.Get(key) }

// Get typed value by key, return zero value if not found
func Get[T any](key string) (T, bool) {
	var zero T // 零值
	val, ok := std.Get(key)
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

// MGet get multiple key-value pairs from the cache.
func MGet(keys ...string) map[string]any { return std.MGet(keys...) }

// MSet set multiple key-value pairs in the cache.
func MSet(items map[string]any, ttl time.Duration) { std.MSet(items, ttl) }

// Keys get the keys of the default cache
func Keys() []string { return std.Keys() }

// Len get the number of items in the cache
func Len() int { return std.Len() }

// Clear all items from the default cache
func Clear() { std.Clear() }

// Delete key
func Delete(key string) { std.Delete(key) }

// SaveFile Save the cache data to a file.
func SaveFile(filename string) error {
	return std.SaveFile(filename)
}

// LoadFile Recover cache data from file load
func LoadFile(filename string) error {
	return std.LoadFile(filename)
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
