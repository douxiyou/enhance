/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"douxiyou.com/enhance/pkg/instance"
	"github.com/spf13/cobra"
)

// serverCmd represents the server command
var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "启动路由器增强工具服务器",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("server called")
		inst := instance.NewInstance()
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sig
			inst.Stop()
		}()
		inst.Start()
	},
}

func init() {
	rootCmd.AddCommand(serverCmd)
}
