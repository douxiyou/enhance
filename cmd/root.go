/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"os"

	serverConfig "douxiyou.com/enhance/pkg/config"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "enhance",
	Short: "路由器增强工具",
	Long:  `路由器增强工具，用于增强路由器的功能`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if err := serverConfig.NewConfig(configDir); err != nil {
			cmd.Println("加载配置失败", zap.Error(err))
			os.Exit(1)
		}
	},
}
var (
	configDir string
)

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&configDir, "config", ".", "配置文件路径")
}
