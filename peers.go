package geecache

// PeerPicker 根据 key 选择节点，返回对应的 PeerGetter 以及是否找到节点
type PeerPicker interface {
	PickPeer(key string) (peer PeerGetter, ok bool)
}

// PeerGetter 提供从对等节点获取数据的方法
type PeerGetter interface {
	Get(group string, key string) ([]byte, error)
}
