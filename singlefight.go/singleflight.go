package singleflight

import "sync"

// call 表示一次请求
type call struct {
	wg  sync.WaitGroup
	val interface{}
	err error
}

// Group 是 singleflight 的主数据结构，管理不同 key 的请求(call)。
type Group struct {
	mu sync.RWMutex // 保护m
	m  map[string]*call
}

// Do 执行 fn 函数，如果 key 对应的请求正在进行中，则等待请求完成。
// 返回值是 fn 函数的返回值。
func (g *Group) Do(key string, fn func() (interface{}, error)) (interface{}, error) {
	g.mu.Lock()
	// 如果 map 未初始化，则创建一个空的 map
	// 延迟创建
	if g.m == nil {
		g.m = make(map[string]*call)
	}
	// 如果 key 对应的 call 已存在，则等待请求完成
	if c, ok := g.m[key]; ok {
		g.mu.Unlock()
		c.wg.Wait()
		return c.val, c.err
	}
	// 如果 key 对应的 call 不存在，则创建新的 call 并执行请求
	c := new(call)
	c.wg.Add(1)
	g.m[key] = c
	g.mu.Unlock()
	c.val, c.err = fn()
	// 请求执行完成，通知等待的 goroutine
	c.wg.Done()
	// 删除 key 对应的 call
	g.mu.Lock()
	delete(g.m, key)
	g.mu.Unlock()
	return c.val, c.err
}
