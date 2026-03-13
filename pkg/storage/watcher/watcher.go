package watcher

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"douxiyou.com/enhance/pkg/config"
	"douxiyou.com/enhance/pkg/storage"
	"go.uber.org/zap"
)

type Watcher[T any] struct {
	entries     map[string]T
	mutex       sync.RWMutex
	log         *zap.Logger
	constructor func(*storage.KeyValue) (T, error)
	prefix      *storage.Key
	client      *storage.Client

	withPrefix       bool
	afterInitialLoad func()
	beforeUpdate     func(entry T, direction storage.EventType)

	keyFunc func(string) string

	watchCancel context.CancelFunc
}

func New[T any](
	constructor func(*storage.KeyValue) (T, error),
	client *storage.Client,
	prefix *storage.Key,
	opts ...func(w *Watcher[T]),
) *Watcher[T] {
	w := &Watcher[T]{
		entries:     make(map[string]T),
		mutex:       sync.RWMutex{},
		log:         config.GetGlobalConfig().Logger().Named("storage.watcher").With(zap.String("prefix", prefix.String())),
		constructor: constructor,
		prefix:      prefix,
		client:      client,
		withPrefix:  false,
		keyFunc: func(s string) string {
			return s
		},
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

func (w *Watcher[T]) Prefix() *storage.Key {
	return w.prefix.Copy().Prefix(false)
}

func (w *Watcher[T]) Start(ctx context.Context) {
	w.log.Debug("启动 watcher")
	w.loadInitial(ctx)
	cctx, cancel := context.WithCancel(ctx)
	w.watchCancel = cancel
	go w.startWatch(cctx)
}

func (w *Watcher[T]) Stop(ctx context.Context) {
	w.log.Debug("停止 watcher")
	if w.watchCancel != nil {
		w.watchCancel()
	}
	if w.beforeUpdate == nil {
		return
	}
	w.mutex.RLock()
	for _, e := range w.entries {
		w.beforeUpdate(e, storage.DELETE)
	}
	w.mutex.RUnlock()
}

func (w *Watcher[T]) loadInitial(ctx context.Context) {
	w.log.Debug("加载初始数据")
	entries, err := w.client.Get(ctx, w.prefix.String(), storage.WithPrefix())
	if err != nil {
		w.log.Warn("列出条目失败", zap.Error(err))
		if !errors.Is(err, context.Canceled) {
			select {
			case <-time.After(1 * time.Second):
				w.loadInitial(ctx)
			case <-ctx.Done():
				return
			}
		}
		return
	}
	for _, entry := range entries.Kvs {
		w.handleEvent(storage.PUT, entry)
	}
	if w.afterInitialLoad != nil {
		w.afterInitialLoad()
	}
}

func (w *Watcher[T]) startWatch(ctx context.Context) {
	ch := w.client.Watch(ctx, w.prefix.String(), storage.WithPrefix())
	for watchResp := range ch {
		for _, event := range watchResp.Events {
			w.handleEvent(event.Type, event.Kv)
		}
	}
}

func (w *Watcher[T]) handleEvent(t storage.EventType, kv *storage.KeyValue) bool {
	key := w.keyFunc(string(kv.Key))
	relKey := strings.TrimPrefix(key, w.prefix.String())
	if !w.withPrefix && strings.Contains(relKey, "/") {
		return false
	}
	if w.beforeUpdate != nil {
		w.mutex.RLock()
		old := w.entries[key]
		w.beforeUpdate(old, t)
		w.mutex.RUnlock()
	}
	switch t {
	case storage.DELETE:
		w.log.Debug("移除条目", zap.String("key", key))
		w.mutex.Lock()
		defer w.mutex.Unlock()
		delete(w.entries, key)
	case storage.PUT:
		e, err := w.constructor(kv)
		if err != nil {
			w.log.Warn("构造条目失败", zap.Error(err))
			return false
		}
		w.mutex.Lock()
		w.entries[key] = e
		w.mutex.Unlock()
		w.log.Debug("添加条目", zap.String("key", key))
	}
	return true
}
