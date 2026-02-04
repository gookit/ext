package lcache

import (
	"container/list"
	"errors"
	"os"
	"sync"
	"time"

	"github.com/gookit/goutil/fsutil"
	"github.com/gookit/goutil/x/stdio"
)

// Item represents a cached item with expiration
type Item struct {
	Val any `json:"v"`
	// 过期时间 millitime. 0表示永不过期
	Exp int64 `json:"e"`
}

// isExpired 检查是否已过期
func (i *Item) isExpired() bool {
	if i.Exp == 0 {
		return false
	}
	return time.Now().UnixMilli() > i.Exp
}

func (i *Item) isExpired1(nowUm int64) bool {
	if i.Exp == 0 {
		return false
	}
	return nowUm > i.Exp
}

// Cache represents a thread-safe local cache with TTL support
type Cache struct {
	opt Options
	mu  sync.RWMutex // 读写锁
	// 存储 key-value map
	items map[string]*Item
	// LRU 链表管理访问顺序
	lruList *list.List
	lruMap  map[string]*list.Element // LRU 链表节点索引，用于快速删除
}

// New create a new cache instance with options
func New(optFns ...OptionFn) *Cache {
	c := &Cache{
		items:   make(map[string]*Item),
		lruList: list.New(),
		lruMap:  make(map[string]*list.Element),
		// options
		opt: Options{
			Capacity:   1000,
			Serializer: "json",
		},
	}

	return c.Configure(optFns...)
}

// Configure the cache instance with options.
func (c *Cache) Configure(optFns ...OptionFn) *Cache {
	for _, optFn := range optFns {
		optFn(&c.opt)
	}
	return c
}

// Set adds an item to the cache with a specified duration.
// If duration <= 0, the item will never Exp.
func (c *Cache) Set(key string, value any, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var exp int64
	if ttl > 0 {
		exp = time.Now().Add(ttl).UnixMilli()
	}

	// 如果 key 已存在，更新值并移动到 LRU 头部
	if elem, ok := c.lruMap[key]; ok {
		c.lruList.MoveToFront(elem)
		c.items[key] = &Item{Val: value, Exp: exp}
		return
	}

	// 检查容量并执行淘汰
	if c.lruList.Len() >= c.opt.Capacity {
		c.evict()
	}

	// 添加新项
	c.items[key] = &Item{Val: value, Exp: exp}
	elem := c.lruList.PushFront(key)
	c.lruMap[key] = elem
}

// Val get value by key, not return exists
func (c *Cache) Val(key string) any {
	val, _ := c.Get(key)
	return val
}

// Get retrieves an item from the cache.
// Returns the Val and true if found and not expired, otherwise nil and false.
func (c *Cache) Get(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	it, ok := c.items[key]
	if !ok {
		return nil, false
	}

	// 检查过期
	if it.isExpired() {
		c.removeElement(key)
		return nil, false
	}

	// 更新 LRU 位置
	if elem, ok := c.lruMap[key]; ok {
		c.lruList.MoveToFront(elem)
	}
	return it.Val, true
}

// MGet get the values corresponding to multiple keys in batches
func (c *Cache) MGet(keys ...string) map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()

	result := make(map[string]any, len(keys))
	nowUm := time.Now().UnixMilli()

	for _, key := range keys {
		it, ok := c.items[key]
		if !ok || it.isExpired1(nowUm) {
			result[key] = nil
			continue
		}

		// 更新 LRU 位置
		if elem, ok := c.lruMap[key]; ok {
			c.lruList.MoveToFront(elem)
		}
		result[key] = it.Val
	}

	return result
}

// MSet set multiple key-value pairs in bulk
func (c *Cache) MSet(items map[string]any, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var exp int64
	if ttl > 0 {
		exp = time.Now().Add(ttl).UnixMilli()
	}

	for key, value := range items {
		// 如果 key 已存在，更新值并移动到 LRU 头部
		if elem, ok := c.lruMap[key]; ok {
			c.lruList.MoveToFront(elem)
			c.items[key] = &Item{Val: value, Exp: exp}
			continue
		}

		// 检查容量并执行淘汰
		if c.lruList.Len() >= c.opt.Capacity {
			c.evict()
		}

		// 添加新项
		c.items[key] = &Item{Val: value, Exp: exp}
		elem := c.lruList.PushFront(key)
		c.lruMap[key] = elem
	}
}

// Has checks if an item exists in the cache.
func (c *Cache) Has(key string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.items[key] != nil
}

// Keys Get a list of all valid keys in the current cache
//
// 注意：此操作会遍历所有数据，时间复杂度为 O(N)
// 如果数据量巨大，可能会短暂阻塞写操作
func (c *Cache) Keys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	keys := make([]string, 0, len(c.items))
	nowUm := time.Now().UnixMilli()

	// 遍历 map 过滤掉已过期的 key
	for k, v := range c.items {
		if !v.isExpired1(nowUm) {
			keys = append(keys, k)
		}
	}

	return keys
}

// Len get the number of items in the cache
//
// 返回的是 map 的大小，包含可能已过期但尚未被清理的“僵尸”数据
// 为了保证 O(1) 的高性能，这里不进行遍历去重
func (c *Cache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

// Clear removes all items from the cache
//
// 这会重置底层的 map 和 list，释放内存引用
// 注意：这不会触发 onEvicted 回调函数，因为那是针对单个元素淘汰的
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reset()
}

// 直接重新初始化，比逐个 Delete 效率高得多
func (c *Cache) reset() {
	c.items = make(map[string]*Item)
	c.lruMap = make(map[string]*list.Element)
	c.lruList.Init()
}

// Delete removes an item from the cache
func (c *Cache) Delete(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.removeElement(key)
}

// removeElement 内部删除方法 (不加锁)
func (c *Cache) removeElement(key string) (exists bool) {
	if elem, ok := c.lruMap[key]; ok {
		c.lruList.Remove(elem)
		delete(c.lruMap, key)
		exists = true
	}

	if it, ok := c.items[key]; ok {
		exists = true
		delete(c.items, key)
		if c.opt.OnEvicted != nil {
			c.opt.OnEvicted(key, it.Val)
		}
	}
	return
}

// evict 淘汰最久未使用的项
func (c *Cache) evict() {
	elem := c.lruList.Back()
	if elem != nil {
		key := elem.Value.(string)
		c.removeElement(key)
	}
}

// getSerializer 获取序列化器
func (c *Cache) serializer() (Serializer, error) {
	if serializer, ok := serializers[c.opt.Serializer]; ok {
		return serializer, nil
	}
	return nil, errors.New("not registered serializer: " + c.opt.Serializer)
}

// SaveFile Save the cache data to a file.
func (c *Cache) SaveFile(filename string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// 准备序列化数据，剔除已过期的
	data := make(map[string]any)
	nowUm := time.Now().UnixMilli()
	for k, v := range c.items {
		if !v.isExpired1(nowUm) {
			data[k] = v
		}
	}

	if len(data) == 0 {
		return nil
	}

	file, err := fsutil.OpenTruncFile(filename, 0644)
	if err != nil {
		return err
	}
	defer stdio.SafeClose(file)

	serializer, err1 := c.serializer()
	if err1 != nil {
		return err1
	}

	return serializer.EncodeTo(file, data)
}

// LoadFile Recover cache data from file load
func (c *Cache) LoadFile(filename string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer stdio.SafeClose(file)

	serializer, err1 := c.serializer()
	if err1 != nil {
		return err1
	}

	var data map[string]Item
	err = serializer.DecodeFrom(file, &data)
	if err != nil {
		return err
	}

	// 恢复数据 (清空当前数据)
	c.reset()
	nowUm := time.Now().UnixMilli()

	for k, v := range data {
		// 加载时检查是否过期，避免加载即过期
		if !v.isExpired1(nowUm) {
			c.items[k] = &v
			elem := c.lruList.PushFront(k)
			c.lruMap[k] = elem
		}
	}

	return nil
}

