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
	sm.log.Info("service manager start")
	sm.serviceMutex.Lock()
	if _, ok := sm.services[serviceKey]; ok {
		sm.serviceMutex.Unlock()
		sm.log.Info("service already running", zap.String("service", string(serviceKey)))
		return nil
	}
	sm.serviceMutex.Unlock()
	sctx, cancel := context.WithCancelCause(sm.rootContext)
	sc := ServiceContext{
		ServiceInstance: sm.ForService(string(serviceKey), sctx),
		cancelFunc:      cancel,
	}
	sc.Service = services.GetService(string(serviceKey))(sc.ServiceInstance)
	sm.serviceMutex.Lock()
	sm.services[serviceKey] = sc
	sm.serviceMutex.Unlock()
	serviceCtx, ok := sm.services[serviceKey]
	if !ok {
		sm.log.Error("service not found", zap.String("service", string(serviceKey)))
		return nil
	}
	go func() {
		err := serviceCtx.Service.Start(serviceCtx.ServiceInstance.Context())
		if err != nil {
			sm.log.Error("start service failed", zap.Error(err))
		}
	}()
	return nil
}
func (sm *ServiceManager) StopService(serviceKey ServiceKey) error {
	sm.serviceMutex.Lock()
	serviceCtx, ok := sm.services[serviceKey]
	sm.log.Info("services dhcp", zap.Any("service", serviceCtx.Service))
	if !ok {
		sm.serviceMutex.Unlock()
		sm.log.Debug("service already stopped", zap.String("service", string(serviceKey)))
		return nil
	}
	delete(sm.services, serviceKey)
	sm.serviceMutex.Unlock()

	err := serviceCtx.Service.Stop(serviceCtx.ServiceInstance.Context())
	if err != nil {
		sm.log.Error("stop service failed", zap.Error(err))
		return err
	}
	serviceCtx.cancelFunc(err)
	return nil
}
func (sm *ServiceManager) Done() {
	<-sm.rootContext.Done()
}
