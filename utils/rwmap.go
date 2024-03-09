package utils

import (
	"sync"
)

type RWMap struct { // 一个读写锁保护的线程安全的map
	mu sync.RWMutex // 读写锁保护下面的map字段
	m  map[string]bool
}

func NewRWMap() *RWMap {
	return &RWMap{
		m: make(map[string]bool, 0),
	}
}

func (m *RWMap) Get(k string) (bool, bool) { //从map中读取一个值
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, existed := m.m[k] // 在锁的保护下从map中读取
	return v, existed
}

func (m *RWMap) Set(k string, v bool) { // 设置一个键值对
	m.mu.Lock() // 锁保护
	defer m.mu.Unlock()
	m.m[k] = v
}

func (m *RWMap) SetNX(k string) bool { // 设置一个键值对，如果键存在则返回 false
	m.mu.Lock() // 锁保护
	defer m.mu.Unlock()

	_, existed := m.m[k]
	if existed {
		return false
	}
	m.m[k] = true
	return true
}

func (m *RWMap) Delete(k string) { //删除一个键
	m.mu.Lock() // 锁保护
	defer m.mu.Unlock()
	delete(m.m, k)
}

func (m *RWMap) Len() int { // map的长度
	m.mu.RLock() // 锁保护
	defer m.mu.RUnlock()
	return len(m.m)
}

func (m *RWMap) Each(f func(k string, v bool) bool) { // 遍历map
	m.mu.RLock() //遍历期间一直持有读锁
	defer m.mu.RUnlock()

	for k, v := range m.m {
		if !f(k, v) {
			return
		}
	}
}
