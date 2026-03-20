package watcher

import "fmt"

func (w *Watcher[T]) GetWithPrefix(parts ...string) (T, bool) {
	return w.GetOK(w.Prefix().Add(parts...).String())
}

func (w *Watcher[T]) GetOK(key string) (T, bool) {
	fmt.Println("查找数据时的key:", key)
	w.mutex.RLock()
	defer w.mutex.RUnlock()
	entry, ok := w.entries[key]
	return entry, ok
}
