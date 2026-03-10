package services

import (
	"context"

	"douxiyou.com/enhance/pkg/storage"
	"go.uber.org/zap"
)

type Service interface {
	Name() string
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}
type Instance interface {
	KV() *storage.Client
	Log() *zap.Logger
	Context() context.Context
}
type ServiceConstructor func(Instance) Service

var serviceRegistry map[string]ServiceConstructor = make(map[string]ServiceConstructor)

func RegisterService(name string, constructor ServiceConstructor) {
	serviceRegistry[name] = constructor
}
func GetService(name string) ServiceConstructor {
	return serviceRegistry[name]
}
