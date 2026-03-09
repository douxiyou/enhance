package storage

import (
	"context"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/namespace"
	"go.uber.org/zap"
)

type Client struct {
	*clientv3.Client
	log    *zap.Logger
	config clientv3.Config
	prefix string
	debug  bool
	parent *Client
}

func NewClient(prefix string, logger *zap.Logger, debug bool) *Client {
	config := clientv3.Config{
		DialTimeout:          2 * time.Second,
		DialKeepAliveTime:    2 * time.Second,
		DialKeepAliveTimeout: 2 * time.Second,
		Logger:               logger,
	}
	cli, err := clientv3.New(config)
	if err != nil {
		logger.Fatal("etcd client create failed", zap.Error(err))
	}
	cli.KV = namespace.NewKV(cli.KV, prefix)
	cli.Watcher = namespace.NewWatcher(cli, prefix)
	cli.Lease = namespace.NewLease(cli.Lease, prefix)
	return &Client{
		Client: cli,
		log:    logger,
		config: config,
		prefix: prefix,
		debug:  debug,
	}
}
func (c *Client) Config() clientv3.Config {
	return c.config
}
func (c *Client) Get(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
	res, err := c.Client.Get(ctx, key, opts...)
	if err != nil {
		return res, err
	}
	return res, nil
}
func (c *Client) Put(ctx context.Context, key string, val string, opts ...clientv3.OpOption) (*clientv3.PutResponse, error) {
	res, err := c.Client.Put(ctx, key, val, opts...)
	if err != nil {
		return res, err
	}
	return res, nil
}
func (c *Client) Delete(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.DeleteResponse, error) {
	res, err := c.Client.Delete(ctx, key, opts...)
	if err != nil {
		return res, err
	}
	return res, nil
}
