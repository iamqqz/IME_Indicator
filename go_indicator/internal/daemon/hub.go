// 订阅者集合与状态变更广播。
package daemon

import "sync"

// Hub 维护订阅者通道，向所有订阅者广播状态变更。
type Hub struct {
	mu      sync.Mutex
	subs    map[chan string]struct{}
	dropped uint64 // 因通道满而丢弃的事件计数
}

// NewHub 创建 Hub
func NewHub() *Hub {
	return &Hub{subs: make(map[chan string]struct{})}
}

// Subscribe 注册订阅，返回事件通道（容量 8，溢出丢弃最旧）
func (h *Hub) Subscribe() chan string {
	ch := make(chan string, 8)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

// Unsubscribe 取消订阅并关闭通道
func (h *Hub) Unsubscribe(ch chan string) {
	h.mu.Lock()
	if _, ok := h.subs[ch]; ok {
		delete(h.subs, ch)
		close(ch)
	}
	h.mu.Unlock()
}

// Dropped 返回因通道满而丢弃的事件总数
func (h *Hub) Dropped() uint64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.dropped
}

// Broadcast 向所有订阅者推送消息（非阻塞）。
// 通道满时丢弃最旧事件（弹出一头）后再塞入最新，并计数。
func (h *Hub) Broadcast(msg string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- msg:
			continue
		default:
		}
		// 满：丢弃最旧（弹出一个未读事件），再尝试塞入最新
		select {
		case <-ch:
			h.dropped++
		default:
		}
		select {
		case ch <- msg:
		default:
			// 极端竞争下仍满，丢弃最新并计数
			h.dropped++
		}
	}
}
