package lru

import "container/list"

// Cache is a simple LRU cache. It is not safe for concurrent access.
type Cache struct {
	// 允许使用的最大内存
	maxBytes int64
	// 当前已使用的内存
	nbytes int64
	// 双向链表，链表的每个节点都是一个entry
	ll    *list.List
	cache map[string]*list.Element
	// Optional and executed when an entry is purged.
	// 某条记录被移除时的回调函数，可以为 nil。
	OnEvicted func(key string, value Value)
}

// 双向链表节点的数据类型，保留key的好处是在删除节点时可以直接去map中删除key
type entry struct {
	key   string
	value Value
}

// Value use Len to count how many bytes it takes
// 这个接口是为了通用性，允许值是任意类型，只要实现了Len()方法即可
type Value interface {
	Len() int
}

// Cache 构造函数
func New(maxBytes int64, onEvicted func(string, Value)) *Cache {
	return &Cache{
		maxBytes:  maxBytes,
		ll:        list.New(),
		cache:     make(map[string]*list.Element),
		OnEvicted: onEvicted,
	}
}

// 查找功能
// 查找操作会使得对应的节点移动到队尾
func (c *Cache) Get(key string) (value Value, ok bool) {
	if ele, ok := c.cache[key]; ok {
		c.ll.MoveToFront(ele)
		kv := ele.Value.(*entry)
		return kv.value, ok
	}
	return nil, false
}

// 删除是缓存淘汰策略的一部分，删除最久未使用的节点
// 删除操作会维护当前已使用的内存大小，并且调用回调函数
func (c *Cache) RemoveOldest() {
	if ele := c.ll.Back(); ele != nil {
		// 删除链表中的节点
		c.ll.Remove(ele)
		kv := ele.Value.(*entry)
		// 删除map中的key
		delete(c.cache, kv.key)
		// 维护当前已使用的内存大小
		c.nbytes -= int64(len(kv.key)) + int64(kv.value.Len())
		// 调用回调函数
		if c.OnEvicted != nil {
			c.OnEvicted(kv.key, kv.value)
		}
	}
}

// 新增/修改功能

func (c *Cache) Add(key string, value Value) {
	// 如果缓存中存在该key，则修改对应节点的值，并将该节点移动到队头
	if ele, ok := c.cache[key]; ok {
		c.ll.MoveToFront(ele)
		kv := ele.Value.(*entry)
		c.nbytes += int64(value.Len()) - int64(kv.value.Len())
		kv.value = value
	} else {
		// 如果缓存中不存在该key，则新增节点到队头
		ele := c.ll.PushFront(&entry{key, value})
		c.cache[key] = ele
		// 维护当前已使用的内存大小
		c.nbytes += int64(len(key)) + int64(value.Len())
	}
	for c.maxBytes != 0 && c.nbytes > c.maxBytes {
		// RemoveOldest()方法会维护当前已使用的内存大小，并且调用回调函数
		c.RemoveOldest()
	}
}

// 返回缓存中记录的数量
func (c *Cache) Len() int {
	return c.ll.Len()
}
