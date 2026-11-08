# GeeCache

一个用 Go 实现的**分布式缓存系统**，参考 [7 天用 Go 从零实现分布式缓存 GeeCache](https://geektutu.com/post/geecache.html) 教程编写。

它在一个进程中用**分层设计**实现了缓存的完整链路：LRU 淘汰 → 并发安全 → 分组调度 → 节点间通信，为后续扩展成真正的多机分布式缓存打下基础。

## 特性

- **LRU 缓存淘汰**：基于 `container/list` 实现的双向链表 + map，读写均为 O(1)
- **并发安全封装**：为 LRU 套上互斥锁，支持多 goroutine 并发访问
- **一致性哈希**：虚拟节点 + CRC32，节点增删只影响相邻的一小段 key
- **singleflight 防击穿**：同一 key 的并发请求合并为一次回源，避免缓存击穿
- **HTTP 节点通信**：多节点之间通过 HTTP 协议互相取数
- **只读字节视图 ByteView**：对外返回副本，防止缓存值被外部篡改

## 架构分层

```
┌─────────────────────────────────────────┐
│  Group（调度层）   geecache.go            │
│  查缓存 → 未命中 → 选节点/回源 → 回填      │
└───────────────┬─────────────────────────┘
                │
┌───────────────▼─────────────────────────┐
│  cache（并发层）  cache.go                │
│  sync.Mutex 包装 LRU，延迟初始化           │
└───────────────┬─────────────────────────┘
                │
┌───────────────▼─────────────────────────┐
│  lru.Cache（存储/淘汰层）  lru/lru.go     │
│  双向链表 + map，维护最近/最久使用顺序      │
└─────────────────────────────────────────┘
```

辅助模块：

| 模块 | 职责 |
|---|---|
| `consistenthash/` | 一致性哈希，负责「key 归哪个节点管」 |
| `singleflight` | 并发请求合并，防缓存击穿 |
| `http.go` + `peers.go` | 节点间通信与节点选择接口 |

## 目录结构

```
geecache/
├── byteview.go             # ByteView：不可变的字节视图
├── cache.go                # cache：并发安全的缓存封装（互斥锁 + 延迟初始化）
├── geecache.go             # Group：核心调度，缓存查询/回源/回填流程
├── http.go                 # HTTPPool：节点间 HTTP 通信（服务端 + 客户端）
├── peers.go                # PeerPicker / PeerGetter 接口定义
├── geecache_test.go        # 核心逻辑测试
├── go.mod
│
├── lru/
│   ├── lru.go              # LRU 缓存（container/list 实现）
│   └── lru_test.go
│
├── consistenthash/
│   ├── consistenthash.go   # 一致性哈希（虚拟节点 + CRC32）
│   └── consistenthash_test.go
│
├── singlefight.go/
│   └── singleflight.go     # singleflight：合并并发请求
│
└── main/
    ├── main.go             # 演示程序：3 节点 + API 服务
    └── run.sh              # 一键启动脚本
```

> 注：`singlefight.go/` 目录名目前有拼写问题（应为 `singleflight/`，且目录名不应带 `.go` 后缀），建议后续重命名。

## 快速开始

### 一键运行完整 demo

启动 3 个缓存节点（8001 / 8002 / 8003），其中 8003 同时开启 API 服务（9999 端口）：

```bash
cd main
./run.sh
```

脚本会依次：

1. 编译并启动 3 个缓存节点
2. 通过 API 服务并发查询 `key=Tom` 三次
3. 输出类似结果：

```
[Server http://localhost:8003] Pick peer http://localhost:8001
[Server http://localhost:8001] GET /_geecache/scores/Tom
[SlowDB] search key Tom
630630630
```

### 手动分节点启动

```bash
# 三个终端分别启动三个缓存节点
go run ./main -port=8001
go run ./main -port=8002
go run ./main -port=8003 -api=1

# 通过 API 节点查询
curl "http://localhost:9999/api?key=Tom"
```

## 核心设计

### 1. Getter 接口与函数适配器

`Getter` 抽象了「数据源」，缓存未命中时通过它回源。`GetterFunc` 让一个普通函数（尤其是闭包）也能直接充当 `Getter`：

```go
var getter Getter = GetterFunc(func(key string) ([]byte, error) {
    return db.Query(key)
})
```

### 2. ByteView 只读保护

缓存值用 `ByteView` 包装，对外只提供返回**副本**的 `ByteSlice()`，配合 `cloneBytes` 防止外部修改缓存内部数据。

### 3. 一致性哈希与虚拟节点

每个节点在哈希环上放 50 个虚拟节点，key 通过 CRC32 哈希后「顺时针」找到第一个节点。节点增删只影响相邻的一小段 key，避免大规模缓存失效。

### 4. singleflight 防击穿

同一个 key 的并发请求，只有一个真正去回源，其余 goroutine 等待并共享结果（用 `sync.WaitGroup` 实现），防止缓存击穿把后端打爆。

### 5. 防自环转发

`PickPeer` 中 `peer != p.self` 判断：如果 key 的归属节点是自己，就不再转发，直接本地回源，避免「自己转发给自己」的无限循环。

## 一次 Get 的完整流程

```
curl → API 节点 gee.Get(key)
         ├─ 本地缓存命中？ → 直接返回
         └─ 未命中 → load(key)
                     └─ singleflight.Do 合并并发
                         ├─ PickPeer(key) 选节点
                         │    ├─ 命中别的节点 → 远程取数
                         │    └─ 命中自己 → 本地回源 getter.Get(key)
                         └─ 结果回填缓存 → 返回
```

## 环境要求

- Go 1.26+
- 纯标准库实现，无第三方依赖
