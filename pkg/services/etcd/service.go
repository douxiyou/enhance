package etcd

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"douxiyou.com/enhance/pkg/config"
	"douxiyou.com/enhance/pkg/services"
	"go.etcd.io/etcd/server/v3/embed"
	"go.uber.org/zap"
)

const (
	relInstCertPath = "/instance.pem"
	relInstKeyPath  = "/instance_key.pem"
)

func init() {
	services.RegisterService("etcd", func(i services.Instance) services.Service {
		return NewEtcdService(i)
	})
}

type Service struct {
	s services.Instance

	e       *embed.Etcd
	cfg     *embed.Config
	log     *zap.Logger
	etcdDir string
	certDir string
}

func urlMustParse(raw string) url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	return *u
}
func NewEtcdService(i services.Instance) *Service {
	dataDir := config.GetGlobalConfig().DataDir
	etcdDir := filepath.Join(dataDir, "etcd") // 用 filepath.Join 替代字符串拼接，跨平台更友好
	certDir := filepath.Join(dataDir, "cert")
	// MkdirAll 会自动判断目录是否存在，无需提前 Stat
	if err := os.MkdirAll(etcdDir, 0755); err != nil {
		panic(fmt.Sprintf("创建etcd目录失败: %v", err))
	}
	if err := os.MkdirAll(certDir, 0755); err != nil {
		panic(fmt.Sprintf("创建证书目录失败: %v", err))
	}
	// 初始化 embed 配置（带默认值，更易用）
	cfg := embed.NewConfig()
	s := &Service{
		s:       i,
		cfg:     cfg,
		log:     i.Log(),
		etcdDir: etcdDir,
		certDir: certDir,
	}
	// 设置 embed 配置
	cfg.Dir = etcdDir
	cfg.Name = "etcd_default"
	cfg.ZapLoggerBuilder = embed.NewZapLoggerBuilder(i.Log())
	cfg.AutoCompactionMode = "periodic"
	cfg.AutoCompactionRetention = "60m"
	clientURL, _ := url.Parse("https://127.0.0.1:2379")
	peerURL, _ := url.Parse("https://127.0.0.1:2380")
	cfg.ListenClientUrls = []url.URL{*clientURL}
	cfg.ListenPeerUrls = []url.URL{*peerURL}
	cfg.AdvertiseClientUrls = []url.URL{
		urlMustParse("https://localhost:2379"),
	}
	cfg.AdvertisePeerUrls = []url.URL{
		urlMustParse(fmt.Sprintf("https://%s", peerURL.Host)),
	}
	cfg.InitialCluster = fmt.Sprintf("%s=https://%s", cfg.Name, peerURL.Host)
	// cfg.InitialClusterState = "new"
	cfg.ClusterState = "new"
	cfg.PeerAutoTLS = true
	// cfg.PeerTLSInfo.ClientCertFile = path.Join(certDir, "peer", relInstCertPath)
	// cfg.PeerTLSInfo.ClientKeyFile = path.Join(certDir, "peer", relInstKeyPath)
	// cfg.PeerTLSInfo.ClientCertAuth = true
	cfg.SelfSignedCertValidity = 1
	cfg.MaxRequestBytes = 10 * 1024 * 1024 // 10 MB
	return s
}
func (s *Service) Start(ctx context.Context) error {
	start := time.Now()
	e, err := embed.StartEtcd(s.cfg)
	if err != nil {
		return err
	}
	s.e = e
	go func() {
		err := <-e.Err()
		if err != nil {
			s.log.Warn("failed to start/stop etcd", zap.Error(err))
		}
		s.e = nil
	}()
	<-e.Server.ReadyNotify()
	s.log.Info("etcd server is ready", zap.Duration("duration", time.Since(start)))
	return nil
}
func (s *Service) Stop(ctx context.Context) error {
	start := time.Now()
	if s.e != nil {
		s.e.Server.Stop()
		s.e.Close()
		// 等待 etcd 服务完全关闭
		<-s.e.Err()
		s.e = nil
	}
	s.log.Info("etcd server is stopped", zap.Duration("duration", time.Since(start)))
	return nil
}
func (s *Service) Name() string {
	return "etcd"
}
