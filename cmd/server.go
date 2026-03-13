package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"douxiyou.com/enhance/pkg/service_manager"
	"github.com/spf13/cobra"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "启动路由器增强工具服务器",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("服务器启动")
		inst := service_manager.NewServiceManager()
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

		fmt.Println("启动 DHCP 服务...")
		if err := inst.StartService(service_manager.DhcpKey); err != nil {
			fmt.Printf("启动 DHCP 服务失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("DHCP 服务启动成功")

		fmt.Println("服务器运行中。按 Ctrl+C 停止...")
		<-sig

		fmt.Println("停止 DHCP 服务...")
		inst.StopService(service_manager.DhcpKey)
		fmt.Println("DHCP 服务已停止")

		fmt.Println("服务器已停止")
	},
}

func init() {
	rootCmd.AddCommand(serverCmd)
}
