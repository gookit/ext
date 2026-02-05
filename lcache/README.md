# LCache

![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/gookit/ext?style=flat-square)
[![Go Report Card](https://goreportcard.com/badge/github.com/gookit/ext/lcache)](https://goreportcard.com/report/github.com/gookit/ext/lcache)
[![Go Reference](https://pkg.go.dev/badge/github.com/gookit/ext.svg)](https://pkg.go.dev/github.com/gookit/ext/lcache)

Simple, goroutine-safe local cache package with TTL expiration support, LRU eviction policy, and file persistence.

> **[中文说明](README.zh-CN.md)**

## Features

- Simple lightweight key-value caching
- Thread-safe access with read-write locks
- TTL (Time-To-Live) expiration support
- Package-level methods with generic value support
- Automatic cleanup of expired data
- Built-in LRU eviction policy to prevent memory overflow
- Support for persistence to single file and recovery from file
- Configurable cache capacity and eviction callbacks
- Multiple serialization formats support (JSON by default)

## Install

```bash
go get github.com/gookit/ext/lcache
```

## Quick Start

### Basic Usage

```go
package main

import (
    "fmt"
    "time"
    "github.com/gookit/ext/lcache"
)

func main() {
    // Set values with TTL
    lcache.Set("name", "John", 5*time.Minute)
    lcache.Set("age", 30, 0) // Never expires
    
    // Get values
    if name, ok := lcache.Get[string]("name"); ok {
        fmt.Printf("Name: %s\n", name)
    }
    
    if age, ok := lcache.Get[int]("age"); ok {
        fmt.Printf("Age: %d\n", age)
    }
    
    // Get any type (returns interface{})
    if val, ok := lcache.Any("name"); ok {
        fmt.Printf("Value: %v\n", val)
    }
}
```

### Using Cache Instance

```go
package main

import (
    "fmt"
    "time"
    "github.com/gookit/ext/lcache"
)

func main() {
    // Create custom cache instance
    cache := lcache.New(
        lcache.WithCapacity(1000),
        lcache.WithOnEvictFn(func(key string, value any) {
            fmt.Printf("Evicted: %s = %v\n", key, value)
        }),
    )
    defer cache.Clear()
    
    // Set and get values
    cache.Set("key1", "value1", time.Hour)
    cache.Set("key2", 42, 30*time.Minute)
    
    if val, ok := cache.Get("key1"); ok {
        fmt.Printf("Got: %v\n", val)
    }
    
    // Batch operations
    cache.MSet(map[string]any{
        "batch1": "data1",
        "batch2": "data2",
    }, time.Hour)
    
    results := cache.MGet("batch1", "batch2", "missing")
    fmt.Printf("Batch results: %+v\n", results)
}
```

## Advanced Usage

### Custom Eviction Callback

```go
cache := lcache.New(
    lcache.WithCapacity(100),
    lcache.WithOnEvictFn(func(key string, value any) {
        // Log eviction or perform cleanup
        log.Printf("Evicted key: %s, value: %v", key, value)
    }),
)
```

### File Persistence

```go
// Save cache to file
err := lcache.SaveFile("cache.json")
if err != nil {
    log.Fatal(err)
}

// Load cache from file
err = lcache.LoadFile("cache.json")
if err != nil {
    log.Fatal(err)
}
```

### Working with Structs

```go
type User struct {
    ID   int    `json:"id"`
    Name string `json:"name"`
}

// Store struct
user := User{ID: 1, Name: "John"}
lcache.Set("user:1", user, time.Hour)

// Retrieve struct
if u, ok := lcache.Get[User]("user:1"); ok {
    fmt.Printf("User: %+v\n", u)
}
```

### Expiration Handling

```go
// Set short TTL for temporary data
lcache.Set("temp", "temporary data", 5*time.Second)

// Set no expiration for permanent data
lcache.Set("perm", "permanent data", 0)

// Check if data exists before accessing
if val, ok := lcache.Get[string]("temp"); ok {
    fmt.Println("Found:", val)
} else {
    fmt.Println("Data expired or not found")
}
```

## API Methods

### Package Level Functions

#### Data Operations

```go
// Set value with TTL
func Set[T any](key string, val T, ttl time.Duration)
// Get typed value (returns zero value if not found)
func Get[T any](key string) (T, bool)
// Get any type value
func Any(key string) (any, bool)
// Get value without type assertion (returns interface{})
func Val(key string) any
// Delete key
func Delete(key string)
// Check if key exists
func Has(key string) bool
// Get all valid keys
func Keys() []string
// Get cache length
func Len() int
// Clear all items
func Clear()
```

#### Batch Operations

```go
// Get multiple values
func MGet(keys ...string) map[string]any
// Set multiple values
func MSet(items map[string]any, ttl time.Duration)
```

#### Persistence

```go
// Save cache to file
func SaveFile(filename string) error
// Load cache from file
func LoadFile(filename string) error
```

#### Configuration

```go
// Configure default cache
func Configure(optFns ...OptionFn)

// Reset default cache instance
func Reset()
```

### Cache Instance Methods

#### Constructor

```go
// Create new cache instance
func New(optFns ...OptionFn) *Cache
// Configure existing cache instance
func (c *Cache) Configure(optFns ...OptionFn) *Cache
```

#### Data Operations

```go
// Set value with TTL
func (c *Cache) Set(key string, value any, ttl time.Duration)
// Get value
func (c *Cache) Get(key string) (any, bool)
// Get value without checking existence
func (c *Cache) Val(key string) any
// Delete key
func (c *Cache) Delete(key string) bool
// Check if key exists
func (c *Cache) Has(key string) bool
// Get all valid keys
func (c *Cache) Keys() []string
// Get cache length
func (c *Cache) Len() int
// Clear all items
func (c *Cache) Clear()
```

#### Batch Operations

```go
// Get multiple values
func (c *Cache) MGet(keys ...string) map[string]any
// Set multiple values
func (c *Cache) MSet(items map[string]any, ttl time.Duration)
```

#### Persistence

```go
// Save cache to file
func (c *Cache) SaveFile(filename string) error
// Load cache from file
func (c *Cache) LoadFile(filename string) error
```

### Options

```go
// Set cache capacity
func WithCapacity(capacity int) OptionFn
// Set serializer (default: "json")
func WithSerializer(serializer string) OptionFn
// Set eviction callback function
func WithOnEvictFn(fn func(key string, value any)) OptionFn
```

### Serializers

The package provides built-in JSON serializer and supports custom serializers:

```go
// Set custom serializer
lcache.SetSerializer("custom", MyCustomSerializer{})

// Use custom serializer
lcache.Configure(lcache.WithSerializer("custom"))
```

## Performance Considerations

- Uses read-write mutex for thread safety
- LRU eviction prevents memory leaks
- Expired items are cleaned up lazily during access
- File persistence operations are atomic
- Batch operations are more efficient than individual operations

## Testing

```bash
cd lcache
go test -v
go test -race -cover
```

## License

MIT