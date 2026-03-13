package service_manager

import (
	"context"
	"sync"

	"douxiyou.com/enhance/pkg/config"
	"douxiyou.com/enhance/pkg/services"
	_ "douxiyou.com/enhance/pkg/services/dhcp"
	"douxiyou.com/enhance/pkg/storage"
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
	rootContext       context.Context
	rootContextCancel context.CancelCauseFunc
	services          map[ServiceKey]ServiceContext
	serviceMutex      sync.RWMutex
	kv                *storage.Client
	log               *zap.Logger
	identifier        string
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
		kv:                config.StorageClient(),
		rootContext:       ctx,
		rootContextCancel: cancel,
	}
}
func (sm *ServiceManager) StartService(serviceKey ServiceKey) error {
	if sm.exists(serviceKey) {
		sm.log.Info("服务已经在运行", zap.String("service", string(serviceKey)))
		return nil
	}
	sctx, cancel := context.WithCancelCause(sm.rootContext)
	sc := ServiceContext{
		ServiceInstance: sm.ForService(string(serviceKey), sctx),
		cancelFunc:      cancel,
	}
	sc.Service = services.GetService(string(serviceKey))(sc.ServiceInstance)
	sm.serviceMutex.Lock()
	sm.services[serviceKey] = sc
	sm.serviceMutex.Unlock()
	err := sc.Service.Start(sc.ServiceInstance.Context())
	if err != nil {
		sm.log.Error("启动服务失败", zap.Error(err))
		delete(sm.services, serviceKey)
		return err
	}
	return nil
}
func (sm *ServiceManager) StopService(serviceKey ServiceKey) error {
	sm.serviceMutex.Lock()
	serviceCtx, ok := sm.services[serviceKey]
	sm.log.Info("服务 "+string(serviceKey), zap.Any("service", serviceCtx.Service))
	if !ok {
		sm.serviceMutex.Unlock()
		sm.log.Debug("服务已经停止", zap.String("service", string(serviceKey)))
		return nil
	}
	delete(sm.services, serviceKey)
	sm.serviceMutex.Unlock()

	err := serviceCtx.Service.Stop(serviceCtx.ServiceInstance.Context())
	if err != nil {
		sm.log.Error("停止服务失败", zap.Error(err))
		return err
	}
	serviceCtx.cancelFunc(err)
	return nil
}
func (sm *ServiceManager) exists(serviceKey ServiceKey) bool {
	sm.serviceMutex.RLock()
	defer sm.serviceMutex.RUnlock()
	_, ok := sm.services[serviceKey]
	return ok
}
func (sm *ServiceManager) Done() {
	<-sm.rootContext.Done()
}
