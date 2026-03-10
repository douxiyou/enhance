package service_manager

import (
	"context"
	"sync"

	"douxiyou.com/enhance/pkg/config"
	"douxiyou.com/enhance/pkg/services"
	_ "douxiyou.com/enhance/pkg/services/dhcp"
	"douxiyou.com/enhance/pkg/storage"
	"go.etcd.io/etcd/client/v3/concurrency"
	"go.uber.org/zap"
)

type ServiceContext struct {
	Service         services.Service
	ServiceInstance *ServiceInstance
	cancelFunc      context.CancelCauseFunc
}
type ServiceKey string

const (
	DhcpKey ServiceKey = "dhcp"
)

type ServiceManager struct {
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

func NewServiceManager() *ServiceManager {
	config := config.GetGlobalConfig()
	ctx, cancel := context.WithCancelCause(context.Background())
	log := config.Logger().With(zap.String("component", "service_manager")).Named("service_manager")
	return &ServiceManager{
		services:          make(map[ServiceKey]ServiceContext),
		serviceMutex:      sync.RWMutex{},
		log:               log,
		identifier:        "enhance-service-manager",
		kv:                config.EtcdClient(),
		rootContext:       ctx,
		rootContextCancel: cancel,
	}
}
func (sm *ServiceManager) StartService(serviceKey ServiceKey) error {
	sm.log.Info("service manager start")
	sctx, cancel := context.WithCancelCause(sm.rootContext)
	sc := ServiceContext{
		ServiceInstance: sm.ForService(string(serviceKey), sctx),
		cancelFunc:      cancel,
	}
	sc.Service = services.GetService(string(serviceKey))(sc.ServiceInstance)
	sm.serviceMutex.Lock()
	sm.services[serviceKey] = sc
	sm.serviceMutex.Unlock()
	// 拿到指定的服务上下文
	serviceCtx, ok := sm.services[serviceKey]
	if !ok {
		sm.log.Error("service not found", zap.String("service", string(serviceKey)))
		return nil
	}
	// 调用指定服务的启动方法
	err := serviceCtx.Service.Start(serviceCtx.ServiceInstance.Context())
	if err != nil {
		sm.log.Error("start service failed", zap.Error(err))
		return err
	}
	return nil
}
func (sm *ServiceManager) StopService(serviceKey ServiceKey) error {
	sm.log.Info("service manager stop")
	serviceCtx, ok := sm.services[serviceKey]
	if !ok {
		sm.log.Error("service not found", zap.String("service", string(serviceKey)))
		return nil
	}
	err := serviceCtx.Service.Stop(serviceCtx.ServiceInstance.Context())
	if err != nil {
		sm.log.Error("stop service failed", zap.Error(err))
		return err
	}
	serviceCtx.cancelFunc(err)
	return nil
}
