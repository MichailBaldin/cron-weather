package storage

import (
	"sync"

	"cron-weather/internal/subscription"
)

type Cache struct {
	mu    sync.RWMutex
	items map[int64]subscription.Subscription // key: chat_id
}

func NewCache() *Cache {
	return &Cache{
		items: make(map[int64]subscription.Subscription),
	}
}

func (c *Cache) LoadAll(subs []subscription.Subscription) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[int64]subscription.Subscription, len(subs))
	for _, s := range subs {
		c.items[s.ChatID] = s
	}
}

func (c *Cache) Add(sub subscription.Subscription) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[sub.ChatID] = sub
}

func (c *Cache) Remove(chatID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, chatID)
}

func (c *Cache) GetAll() []subscription.Subscription {
	c.mu.RLock()
	defer c.mu.RUnlock()
	list := make([]subscription.Subscription, 0, len(c.items))
	for _, s := range c.items {
		list = append(list, s)
	}
	return list
}

func (c *Cache) Get(chatID int64) (subscription.Subscription, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	s, ok := c.items[chatID]
	return s, ok
}
