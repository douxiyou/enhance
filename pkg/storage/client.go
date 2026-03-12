package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v4"
	"go.uber.org/zap"
)

type Client struct {
	db     *badger.DB
	log    *zap.Logger
	prefix string
	debug  bool
	mu     sync.RWMutex
}

type GetResponse struct {
	Kvs []*KeyValue
}

type KeyValue struct {
	Key   []byte
	Value []byte
}

type PutResponse struct{}

type DeleteResponse struct{}

type WatchChan chan WatchResponse

type WatchResponse struct {
	Events []*Event
}

type Event struct {
	Type EventType
	Kv   *KeyValue
}

type EventType int

const (
	PUT EventType = iota
	DELETE
)

func NewClient(prefix string, dataDir string, logger *zap.Logger, debug bool) *Client {
	badgerDir := filepath.Join(dataDir, "badger")

	opts := badger.DefaultOptions(badgerDir)
	if debug {
		opts.Logger = &badgerLoggerWrapper{logger: logger}
	} else {
		opts.Logger = nil
	}

	db, err := badger.Open(opts)
	if err != nil {
		logger.Fatal("badger client create failed", zap.Error(err))
	}

	return &Client{
		db:     db,
		log:    logger,
		prefix: prefix,
		debug:  debug,
	}
}

type badgerLoggerWrapper struct {
	logger *zap.Logger
}

func (w *badgerLoggerWrapper) Warningf(format string, args ...interface{}) {
	w.logger.Warn(fmt.Sprintf(format, args...))
}

func (w *badgerLoggerWrapper) Infof(format string, args ...interface{}) {
	w.logger.Info(fmt.Sprintf(format, args...))
}

func (w *badgerLoggerWrapper) Debugf(format string, args ...interface{}) {
	w.logger.Debug(fmt.Sprintf(format, args...))
}

func (w *badgerLoggerWrapper) Errorf(format string, args ...interface{}) {
	w.logger.Error(fmt.Sprintf(format, args...))
}

func (c *Client) Get(ctx context.Context, key string, opts ...OpOption) (*GetResponse, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	fullKey := c.getFullKey(key)

	var kvs []*KeyValue

	op := &Op{}
	for _, opt := range opts {
		opt(op)
	}

	if op.prefix {
		txn := c.db.NewTransaction(false)
		defer txn.Discard()

		iterator := txn.NewIterator(badger.DefaultIteratorOptions)
		defer iterator.Close()

		prefix := []byte(fullKey)
		for iterator.Seek(prefix); iterator.ValidForPrefix(prefix); iterator.Next() {
			item := iterator.Item()
			keyCopy := item.KeyCopy(nil)
			valueCopy, err := item.ValueCopy(nil)
			if err != nil {
				c.log.Error("failed to copy value", zap.Error(err))
				continue
			}
			kvs = append(kvs, &KeyValue{
				Key:   keyCopy,
				Value: valueCopy,
			})
		}
	} else {
		txn := c.db.NewTransaction(false)
		defer txn.Discard()

		item, err := txn.Get([]byte(fullKey))
		if err == badger.ErrKeyNotFound {
			return &GetResponse{Kvs: kvs}, nil
		}
		if err != nil {
			return nil, err
		}

		keyCopy := item.KeyCopy(nil)
		valueCopy, err := item.ValueCopy(nil)
		if err != nil {
			return nil, err
		}

		kvs = append(kvs, &KeyValue{
			Key:   keyCopy,
			Value: valueCopy,
		})
	}

	return &GetResponse{Kvs: kvs}, nil
}

func (c *Client) Put(ctx context.Context, key string, val string, opts ...OpOption) (*PutResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	fullKey := c.getFullKey(key)

	err := c.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(fullKey), []byte(val))
	})

	if err != nil {
		return nil, err
	}

	return &PutResponse{}, nil
}

func (c *Client) Delete(ctx context.Context, key string, opts ...OpOption) (*DeleteResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	op := &Op{}
	for _, opt := range opts {
		opt(op)
	}

	if op.prefix {
		fullKey := c.getFullKey(key)
		err := c.db.Update(func(txn *badger.Txn) error {
			iterator := txn.NewIterator(badger.DefaultIteratorOptions)
			defer iterator.Close()

			prefix := []byte(fullKey)
			for iterator.Seek(prefix); iterator.ValidForPrefix(prefix); iterator.Next() {
				item := iterator.Item()
				if err := txn.Delete(item.Key()); err != nil {
					return err
				}
			}
			return nil
		})

		if err != nil {
			return nil, err
		}
	} else {
		fullKey := c.getFullKey(key)
		err := c.db.Update(func(txn *badger.Txn) error {
			return txn.Delete([]byte(fullKey))
		})

		if err != nil {
			return nil, err
		}
	}

	return &DeleteResponse{}, nil
}

func (c *Client) Watch(ctx context.Context, key string, opts ...OpOption) WatchChan {
	ch := make(WatchChan, 100)

	op := &Op{}
	for _, opt := range opts {
		opt(op)
	}

	go c.watchLoop(ctx, key, ch, op.prefix)

	return ch
}

func (c *Client) watchLoop(ctx context.Context, key string, ch WatchChan, prefix bool) {
	defer close(ch)

	fullKey := c.getFullKey(key)

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	lastData := make(map[string][]byte)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			currentData := make(map[string][]byte)

			txn := c.db.NewTransaction(false)

			if prefix {
				iterator := txn.NewIterator(badger.DefaultIteratorOptions)
				prefixBytes := []byte(fullKey)

				for iterator.Seek(prefixBytes); iterator.ValidForPrefix(prefixBytes); iterator.Next() {
					item := iterator.Item()
					keyStr := string(item.KeyCopy(nil))
					value, err := item.ValueCopy(nil)
					if err != nil {
						c.log.Error("failed to copy value", zap.Error(err))
						continue
					}
					currentData[keyStr] = value
				}
				iterator.Close()
			} else {
				item, err := txn.Get([]byte(fullKey))
				if err == nil {
					value, err := item.ValueCopy(nil)
					if err == nil {
						currentData[fullKey] = value
					}
				}
			}

			txn.Discard()

			for keyStr, value := range currentData {
				if lastValue, exists := lastData[keyStr]; !exists || string(lastValue) != string(value) {
					ch <- WatchResponse{
						Events: []*Event{
							{
								Type: PUT,
								Kv: &KeyValue{
									Key:   []byte(keyStr),
									Value: value,
								},
							},
						},
					}
				}
			}

			for keyStr := range lastData {
				if _, exists := currentData[keyStr]; !exists {
					ch <- WatchResponse{
						Events: []*Event{
							{
								Type: DELETE,
								Kv: &KeyValue{
									Key: []byte(keyStr),
								},
							},
						},
					}
				}
			}

			lastData = currentData
		}
	}
}

func (c *Client) getFullKey(key string) string {
	if c.prefix != "" {
		return strings.TrimPrefix(c.prefix, "/") + "/" + strings.TrimPrefix(key, "/")
	}
	return key
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.db != nil {
		return c.db.Close()
	}
	return nil
}

type Op struct {
	prefix bool
}

type OpOption func(*Op)

func WithPrefix() OpOption {
	return func(op *Op) {
		op.prefix = true
	}
}

func (c *Client) Key(parts ...string) *Key {
	return &Key{
		parts: parts,
	}
}

type Key struct {
	parts []string
}

func (k *Key) String() string {
	return "/" + strings.Join(k.parts, "/")
}

func (k *Key) Add(parts ...string) *Key {
	newParts := make([]string, len(k.parts))
	copy(newParts, k.parts)
	newParts = append(newParts, parts...)
	return &Key{parts: newParts}
}

func (k *Key) Copy() *Key {
	newParts := make([]string, len(k.parts))
	copy(newParts, k.parts)
	return &Key{parts: newParts}
}

func (k *Key) Prefix(withLeadingSlash bool) *Key {
	return &Key{
		parts: k.parts,
	}
}

func (c *Client) GetJSON(ctx context.Context, key string, v interface{}, opts ...OpOption) error {
	resp, err := c.Get(ctx, key, opts...)
	if err != nil {
		return err
	}

	if len(resp.Kvs) == 0 {
		return nil
	}

	return json.Unmarshal(resp.Kvs[0].Value, v)
}

func (c *Client) PutJSON(ctx context.Context, key string, v interface{}, opts ...OpOption) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}

	_, err = c.Put(ctx, key, string(data), opts...)
	return err
}
