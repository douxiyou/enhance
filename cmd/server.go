/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"douxiyou.com/enhance/pkg/service_manager"
	"github.com/spf13/cobra"
)

// serverCmd represents the server command
var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "启动路由器增强工具服务器",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("server called")
		inst := service_manager.NewServiceManager()
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sig
			inst.StopService(service_manager.DhcpKey)
		}()
		inst.StartService(service_manager.DhcpKey)
	},
}

func init() {
	rootCmd.AddCommand(serverCmd)
}
