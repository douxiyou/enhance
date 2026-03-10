package service_manager

import (
	"context"

	"douxiyou.com/enhance/pkg/storage"
	"go.uber.org/zap"
)

type ServiceInstance struct {
	kv        *storage.Client
	context   context.Context
	log       *zap.Logger
	parent    *ServiceManager
	serviceId string
}

func (sm *ServiceManager) ForService(serviceId string, ctx context.Context) *ServiceInstance {
	si := &ServiceInstance{
		log:       sm.log.With(zap.String("service", serviceId)),
		serviceId: serviceId,
		parent:    sm,
		context:   ctx,
		kv:        sm.kv,
	}
	return si
}

func (si *ServiceInstance) KV() *storage.Client {
	return si.kv
}

func (si *ServiceInstance) Log() *zap.Logger {
	return si.log
}

func (si *ServiceInstance) Context() context.Context {
	return si.context
}
