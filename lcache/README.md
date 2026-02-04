# lcache

本地内存缓存包，提供简单、协程安全的缓存功能，支持 TTL 过期。

## Features

- 简单轻量级的键值缓存
- 协程安全访问
- 支持 TTL 过期时间
- 包级别的方法，支持泛型值
- 自动清理过期数据
- 内置淘汰策略，避免内存过大
- 支持持久化为单个文件，以及从文件恢复

## Usage

## API

- `New(opts ...OptionFn) *Cache` - 创建缓存实例
- `Set(key string, value any, duration time.Duration)` - 设置缓存值
- `Get(key string) (any, bool)` - 获取值
- `Delete(key string)` - 删除键
- `Has(key string) bool` - 检查键是否存在
- `Keys() []string` - 获取所有未过期键
- `Len() int` - 获取缓存项数量
- `Clear()` - 清空缓存
