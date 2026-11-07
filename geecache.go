package geecache

import (
	"errors"
	"log"
	"sync"
)

// 定义接口Getter和回调函数
type Getter interface {
	Get(key string) ([]byte, error)
}

// GetterFunc 实现接口Getter
type GetterFunc func(key string) ([]byte, error)

// Get 实现接口Getter
func (f GetterFunc) Get(key string) ([]byte, error) {
	return f(key)
}

type Group struct {
	name      string
	getter    Getter
	mainCache cache
}

// 定义全局变量, 用于管理所有的Group实例
var (
	mu    sync.RWMutex
	group = make(map[string]*Group)
)

// NewGroup 创建新的Group
func NewGroup(name string, cacheBytes int64, getter Getter) *Group {
	if getter == nil {
		panic("nil Getter")
	}
	mu.Lock()
	defer mu.Unlock()
	g := &Group{
		name:      name,
		getter:    getter,
		mainCache: cache{cachedBytes: cacheBytes},
	}
	group[name] = g
	return g
}

// GetGroup 获取指定名称的Group
func GetGroup(name string) *Group {
	mu.RLock()
	g := group[name]
	defer mu.RUnlock()
	return g
}

// Get 获取指定key的值
func (g *Group) Get(key string) (ByteView, error) {
	if key == "" {
		return ByteView{}, errors.New("key is required")
	}

	if v, ok := g.mainCache.get(key); ok {
		log.Println("[GeeCache] hit")
		return v, nil
	}
	return g.load(key)
}

// load 加载指定key的值
func (g *Group) load(key string) (value ByteView, err error) {
	return g.getLocally(key)
}

// getLocally 本地加载指定key的值
func (g *Group) getLocally(key string) (ByteView, error) {
	bytes, err := g.getter.Get(key)
	if err != nil {
		return ByteView{}, err
	}
	value := ByteView{b: cloneBytes(bytes)}
	g.populateCache(key, value)
	return value, nil
}

// populateCache 将指定key的值添加到缓存中
func (g *Group) populateCache(key string, value ByteView) {
	g.mainCache.add(key, value)
}
