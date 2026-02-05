# LCache

![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/gookit/ext?style=flat-square)
[![Go Report Card](https://goreportcard.com/badge/github.com/gookit/ext/lcache)](https://goreportcard.com/report/github.com/gookit/ext/lcache)
[![Go Reference](https://pkg.go.dev/badge/github.com/gookit/ext.svg)](https://pkg.go.dev/github.com/gookit/ext/lcache)

简单、协程安全的本地内存缓存包，提供 TTL 过期支持、LRU 淘汰策略和文件持久化功能。

> **[English README](README.md)**

## 功能特性

- 简单轻量级的键值缓存
- 协程安全访问，使用读写锁保护
- 支持 TTL (Time-To-Live) 过期时间
- 包级别的方法，支持泛型值类型
- 自动清理过期数据
- 内置 LRU 淘汰策略，避免内存溢出
- 支持持久化为单个文件，以及从文件恢复
- 可配置缓存容量和淘汰回调函数
- 支持多种序列化格式（默认 JSON）

## 安装

```bash
go get github.com/gookit/ext/lcache
```

## 快速开始

### 基础使用

```go
package main

import (
    "fmt"
    "time"
    "github.com/gookit/ext/lcache"
)

func main() {
    // 设置带 TTL 的值
    lcache.Set("name", "John", 5*time.Minute)
    lcache.Set("age", 30, 0) // 永不过期
    
    // 获取值
    if name, ok := lcache.Get[string]("name"); ok {
        fmt.Printf("姓名: %s\n", name)
    }
    
    if age, ok := lcache.Get[int]("age"); ok {
        fmt.Printf("年龄: %d\n", age)
    }
    
    // 获取任意类型（返回 interface{}）
    if val, ok := lcache.Any("name"); ok {
        fmt.Printf("值: %v\n", val)
    }
}
```

### 使用缓存实例

```go
package main

import (
    "fmt"
    "time"
    "github.com/gookit/ext/lcache"
)

func main() {
    // 创建自定义缓存实例
    cache := lcache.New(
        lcache.WithCapacity(1000),
        lcache.WithOnEvictFn(func(key string, value any) {
            fmt.Printf("被淘汰: %s = %v\n", key, value)
        }),
    )
    defer cache.Clear()
    
    // 设置和获取值
    cache.Set("key1", "value1", time.Hour)
    cache.Set("key2", 42, 30*time.Minute)
    
    if val, ok := cache.Get("key1"); ok {
        fmt.Printf("获取到: %v\n", val)
    }
    
    // 批量操作
    cache.MSet(map[string]any{
        "batch1": "data1",
        "batch2": "data2",
    }, time.Hour)
    
    results := cache.MGet("batch1", "batch2", "missing")
    fmt.Printf("批量结果: %+v\n", results)
}
```

## 高级用法

### 自定义淘汰回调

```go
cache := lcache.New(
    lcache.WithCapacity(100),
    lcache.WithOnEvictFn(func(key string, value any) {
        // 记录淘汰日志或执行清理操作
        log.Printf("淘汰键: %s, 值: %v", key, value)
    }),
)
```

### 文件持久化

```go
// 保存缓存到文件
err := lcache.SaveFile("cache.json")
if err != nil {
    log.Fatal(err)
}

// 从文件加载缓存
err = lcache.LoadFile("cache.json")
if err != nil {
    log.Fatal(err)
}
```

### 处理结构体

```go
type User struct {
    ID   int    `json:"id"`
    Name string `json:"name"`
}

// 存储结构体
user := User{ID: 1, Name: "张三"}
lcache.Set("user:1", user, time.Hour)

// 获取结构体
if u, ok := lcache.Get[User]("user:1"); ok {
    fmt.Printf("用户: %+v\n", u)
}
```

### 过期处理

```go
// 为临时数据设置短 TTL
lcache.Set("temp", "临时数据", 5*time.Second)

// 为永久数据设置无过期时间
lcache.Set("perm", "永久数据", 0)

// 访问前检查数据是否存在
if val, ok := lcache.Get[string]("temp"); ok {
    fmt.Println("找到:", val)
} else {
    fmt.Println("数据已过期或不存在")
}
```

## 测试

```bash
cd lcache
go test -v
go test -race -cover
```

## API 方法

### 包级别函数

#### 数据操作

```go
// 设置带 TTL 的值
func Set[T any](key string, val T, ttl time.Duration)
// 获取指定类型的值（不存在时返回零值）
func Get[T any](key string) (T, bool)
// 获取任意类型值
func Any(key string) (any, bool)
// 获取值但不进行类型断言（返回 interface{}）
func Val(key string) any
// 删除键
func Delete(key string)
// 检查键是否存在
func Has(key string) bool
// 获取所有有效的键
func Keys() []string
// 获取缓存长度
func Len() int
// 清空所有项
func Clear()
```

#### 批量操作

```go
// 获取多个值
func MGet(keys ...string) map[string]any
// 设置多个值
func MSet(items map[string]any, ttl time.Duration)
```

#### 持久化

```go
// 保存缓存到文件
func SaveFile(filename string) error
// 从文件加载缓存
func LoadFile(filename string) error
```

#### 配置

```go
// 配置默认缓存
func Configure(optFns ...OptionFn)
// 重置默认缓存实例
func Reset()
```

### 缓存实例方法

#### 构造函数

```go
// 创建新的缓存实例
func New(optFns ...OptionFn) *Cache
// 配置现有缓存实例
func (c *Cache) Configure(optFns ...OptionFn) *Cache
```

#### 数据操作

```go
// 设置带 TTL 的值
func (c *Cache) Set(key string, value any, ttl time.Duration)
// 获取值
func (c *Cache) Get(key string) (any, bool)
// 获取值但不检查存在性
func (c *Cache) Val(key string) any
// 删除键
func (c *Cache) Delete(key string) bool
// 检查键是否存在
func (c *Cache) Has(key string) bool
// 获取所有有效的键
func (c *Cache) Keys() []string
// 获取缓存长度
func (c *Cache) Len() int
// 清空所有项
func (c *Cache) Clear()
```

#### 批量操作

```go
// 获取多个值
func (c *Cache) MGet(keys ...string) map[string]any
// 设置多个值
func (c *Cache) MSet(items map[string]any, ttl time.Duration)
```

#### 持久化

```go
// 保存缓存到文件
func (c *Cache) SaveFile(filename string) error
// 从文件加载缓存
func (c *Cache) LoadFile(filename string) error
```

### 配置选项

```go
// 设置缓存容量
func WithCapacity(capacity int) OptionFn
// 设置序列化器（默认："json"）
func WithSerializer(serializer string) OptionFn
// 设置淘汰回调函数
func WithOnEvictFn(fn func(key string, value any)) OptionFn
```

### 序列化器

包提供了内置的 JSON 序列化器，并支持自定义序列化器：

```go
// 设置自定义序列化器
lcache.SetSerializer("custom", MyCustomSerializer{})
// 使用自定义序列化器
lcache.Configure(lcache.WithSerializer("custom"))
```

## 许可证

MIT