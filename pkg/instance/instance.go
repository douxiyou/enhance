package instance

import (
	"context"
	"sync"

	"douxiyou.com/enhance/pkg/config"
	"douxiyou.com/enhance/pkg/services"
	"douxiyou.com/enhance/pkg/storage"
	"go.etcd.io/etcd/client/v3/concurrency"
	"go.uber.org/zap"
)

type ServiceContext struct {
	Service           services.Service
	ServiceInstance   *ServiceInstance
	ContextCannelFunc context.CancelFunc
}
type ServiceKey string

const (
	DhcpKey ServiceKey = "dhcp"
)

type Instance struct {
	rootContext context.Context
	// rootContextCancel 根上下文的取消函数
	rootContextCancel context.CancelCauseFunc
	services          map[ServiceKey]ServiceContext
	serviceMutex      sync.RWMutex
	kv                *storage.Client
	log               *zap.Logger
	// identifier 实例标识符
	identifier string
	// instanceSession etcd会话
	instanceSession *concurrency.Session
}

func NewInstance() *Instance {
	config := config.Get()
	ctx, cancel := context.WithCancelCause(context.Background())
	log := config.Logger().With(zap.String("component", "instance")).Named("instance")
	return &Instance{
		services:          make(map[ServiceKey]ServiceContext),
		serviceMutex:      sync.RWMutex{},
		log:               log,
		identifier:        config.Instance.Identifier,
		kv:                config.EtcdClient(),
		rootContext:       ctx,
		rootContextCancel: cancel,
	}
}
func (i *Instance) StartService(ctx context.Context, serviceKey ServiceKey) error {
	i.log.Info("instance start")
	// 启动指定服务
	serviceCtx, ok := i.services[serviceKey]
	if !ok {
		i.log.Error("service not found", zap.String("service", string(serviceKey)))
		return nil
	}
	err := serviceCtx.Service.Start(ctx)
	if err != nil {
		i.log.Error("start service failed", zap.Error(err))
		return err
	}
	return nil
}
func (i *Instance) StopService(ctx context.Context, serviceKey ServiceKey) error {
	i.log.Info("instance stop")
	serviceCtx, ok := i.services[serviceKey]
	if !ok {
		i.log.Error("service not found", zap.String("service", string(serviceKey)))
		return nil
	}
	err := serviceCtx.Service.Stop(ctx)
	if err != nil {
		i.log.Error("stop service failed", zap.Error(err))
		return err
	}
	serviceCtx.ContextCannelFunc()
	return nil
}
