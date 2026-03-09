package instance

import (
	"context"

	"douxiyou.com/enhance/pkg/storage"
	"go.uber.org/zap"
)

type ServiceInstance struct {
	kv        *storage.Client
	context   context.Context
	log       *zap.Logger
	parent    *Instance
	serviceId string
}
