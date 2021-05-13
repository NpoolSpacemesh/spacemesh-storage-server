package main

import (
	"os"

	log "github.com/EntropyPool/entropy-logger"
	"github.com/NpoolChia/chia-storage-server/pkg/mount"
	"github.com/NpoolChia/chia-storage-server/tasks"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

func main() {
	// initMount
	mount.InitMount()

	// 初始化队列
	q := tasks.NewQueue(tasks.DefaultQSize)
	q.AddCallBack(tasks.TaskTodo, tasks.Fetch)    //拉取
	q.AddCallBack(tasks.TaskFinish, tasks.Finsih) //完成
	q.AddCallBack(tasks.TaskFail, tasks.Fail)     //失败

	app := &cli.App{
		Name:                 "chia-storage-service",
		Usage:                "chia storage service",
		Version:              "0.1.0",
		EnableBashCompletion: true,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "config",
				Value: "./chia-storage-service.conf",
			},
		},
		Action: func(cctx *cli.Context) error {
			configFile := cctx.String("config")
			server := NewChiaStorageServer(configFile)
			if server == nil {
				return xerrors.Errorf("can not start chia storage server")
			}
			err := server.Run()
			if err != nil {
				return xerrors.Errorf("fail to run chia storage server: %v", err)
			}

			ch := make(chan int)
			<-ch

			return nil
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatalf(log.Fields{}, "fail to run %v: %v", app.Name, err)
	}
}
