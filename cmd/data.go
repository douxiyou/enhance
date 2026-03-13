/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"log"
	"path/filepath"

	"github.com/dgraph-io/badger/v4"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	databasePath string
	debug        bool
	logInstance  *zap.Logger
)

type badgerLoggerWrapper struct {
	logger *zap.Logger
}

func (w *badgerLoggerWrapper) Warningf(format string, args ...interface{}) {
	w.logger.Warn(fmt.Sprintf(format, args...))
}

func (w *badgerLoggerWrapper) Infof(format string, args ...interface{}) {
	w.logger.Info(fmt.Sprintf(format, args...))
}

func (w *badgerLoggerWrapper) Debugf(format string, args ...interface{}) {
	w.logger.Debug(fmt.Sprintf(format, args...))
}

func (w *badgerLoggerWrapper) Errorf(format string, args ...interface{}) {
	w.logger.Error(fmt.Sprintf(format, args...))
}

// dataCmd represents the data command
var dataCmd = &cobra.Command{
	Use:   "data",
	Short: "A brief description of your command",
	Long:  `查看badger数据`,
	Run: func(cmd *cobra.Command, args []string) {
		badgerDir := filepath.Join(databasePath)
		opts := badger.DefaultOptions(badgerDir)

		if debug {
			opts.Logger = &badgerLoggerWrapper{logger: logInstance}
		} else {
			opts.Logger = nil
		}

		db, err := badger.Open(opts)
		if err != nil {
			logInstance.Fatal("badger client create failed", zap.Error(err))
		}
		defer db.Close()

		err = db.View(func(txn *badger.Txn) error {
			opts := badger.DefaultIteratorOptions
			opts.PrefetchSize = 10
			it := txn.NewIterator(opts)
			defer it.Close()
			for it.Rewind(); it.Valid(); it.Next() {
				item := it.Item()
				k := item.Key()
				err = item.Value(func(val []byte) error {
					fmt.Printf("Key %s, Value %s \n", k, val)
					return nil
				})
				// value, err := item.ValueCopy(nil)
				if err != nil {
					logInstance.Error("badger iterator value copy failed", zap.Error(err))
					continue
				}
				// logInstance.Info("data", zap.Any("key", item.Key()), zap.Any("value", value))
			}
			return nil
		})
		if err != nil {
			logInstance.Fatal("badger iterator failed", zap.Error(err))
		}
	},
}

func init() {
	logInstance = zap.New(zapcore.NewCore(
		zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		zapcore.AddSync(log.Writer()),
		zap.DebugLevel,
	))
	rootCmd.AddCommand(dataCmd)
	dataCmd.Flags().StringVar(&databasePath, "path", "./data/badger", "数据库文件路径")
	dataCmd.Flags().BoolVar(&debug, "debug", true, "badger是否以debug模式运行")
}
