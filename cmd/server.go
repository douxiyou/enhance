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
		fmt.Println("server called")
		inst := service_manager.NewServiceManager()
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		
		fmt.Println("Starting DHCP service...")
		if err := inst.StartService(service_manager.DhcpKey); err != nil {
			fmt.Printf("Failed to start DHCP service: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("DHCP service started successfully")
		
		fmt.Println("Server running. Press Ctrl+C to stop...")
		<-sig
		
		fmt.Println("Stopping DHCP service...")
		inst.StopService(service_manager.DhcpKey)
		fmt.Println("DHCP service stopped")
		
		fmt.Println("Server stopped")
	},
}

func init() {
	rootCmd.AddCommand(serverCmd)
}
