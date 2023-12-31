package main

import (
	"os"

	log "github.com/EntropyPool/entropy-logger"
	"github.com/NpoolSpacemesh/spacemesh-storage-server/tasks"
	"github.com/NpoolSpacemesh/spacemesh-storage-server/util"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

func main() {
	// 初始化队列
	tasks.NewQueue(tasks.DefaultQSize)
	tasks.AddCallBack(tasks.TaskTodo, tasks.Fetch)    //拉取
	tasks.AddCallBack(tasks.TaskFinish, tasks.Finsih) //完成
	tasks.AddCallBack(tasks.TaskFail, tasks.Fail)     //失败

	app := &cli.App{
		Name:                 "spacemesh-storage-service",
		Usage:                "spacemesh storage service",
		Version:              "0.1.0",
		EnableBashCompletion: true,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "config",
				Value: "./spacemesh-storage-server.conf",
			},
		},
		Action: func(cctx *cli.Context) error {
			configFile := cctx.String("config")
			server := NewStorageServer(configFile)
			if server == nil {
				return xerrors.Errorf("can not start spacemesh storage server")
			}

			// init db
			util.InitBoltClient(server.config.DBPath)

			err := server.Run()
			if err != nil {
				return xerrors.Errorf("fail to run spacemesh storage server: %v", err)
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
