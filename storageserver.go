package main

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"

	log "github.com/EntropyPool/entropy-logger"
	httpdaemon "github.com/NpoolRD/http-daemon"
	"github.com/NpoolSpacemesh/spacemesh-storage-server/pkg/mount"
	"github.com/NpoolSpacemesh/spacemesh-storage-server/tasks"
	types "github.com/NpoolSpacemesh/spacemesh-storage-server/types"
	"github.com/NpoolSpacemesh/spacemesh-storage-server/util"
	"github.com/boltdb/bolt"
)

type StorageServerConfig struct {
	Port int `json:"port"`
	// 数据库地址
	DBPath        string `json:"db_path"`
	ClusterName   string `json:"cluster_name"`
	ReservedSpace uint64 `json:"reserved_space"`
}

type StorageServer struct {
	config StorageServerConfig
}

func NewStorageServer(configFile string) *StorageServer {
	buf, err := ioutil.ReadFile(configFile)
	if err != nil {
		log.Errorf(log.Fields{}, "cannot read file %v: %v", configFile, err)
		return nil
	}

	config := StorageServerConfig{}
	err = json.Unmarshal(buf, &config)
	if err != nil {
		log.Errorf(log.Fields{}, "cannot parse file %v: %v", configFile, err)
		return nil
	}

	server := &StorageServer{
		config: config,
	}

	log.Infof(log.Fields{}, "successful to create spacemesh storage server")
	mount.InitMount(config.ReservedSpace)

	return server
}

var (
	errPlotURLEmpty = errors.New("plot url is empty")
)

func (s *StorageServer) UploadPlotRequest(w http.ResponseWriter, req *http.Request) (interface{}, string, int) {
	// get spacemesh plot file
	b, err := ioutil.ReadAll(req.Body)
	if err != nil {
		log.Errorf(log.Fields{}, "fail to read body from %v", req.URL)
		return nil, err.Error(), -1
	}

	input := types.UploadPlotInput{}
	err = json.Unmarshal(b, &input)
	if err != nil {
		log.Errorf(log.Fields{}, "fail to parse body from %v", req.URL)
		return nil, err.Error(), -2
	}
	if input.PlotURL == "" ||
		input.FinishURL == "" ||
		input.FailURL == "" {
		log.Errorf(log.Fields{}, "invalid input parameters from %v", req.URL)
		return nil, errPlotURLEmpty.Error(), -3
	}

	// 入库，调度队列处理
	db, err := util.BoltClient()
	if err != nil {
		return nil, err.Error(), -3
	}

	if err := db.Update(func(tx *bolt.Tx) error {
		bk := tx.Bucket(util.DefaultBucket)
		bk.Delete([]byte(input.PlotURL))
		meta := tasks.Meta{
			Status:      tasks.TaskTodo,
			ClusterName: s.config.ClusterName,
			PlotURL:     input.PlotURL,
			FinishURL:   input.FinishURL,
			FailURL:     input.FailURL,
			DiskSpace:   input.DiskSpace,
		}
		ms, err := json.Marshal(meta)
		if err != nil {
			return err
		}
		return bk.Put([]byte(input.PlotURL), ms)
	}); err != nil {
		return nil, err.Error(), -4
	}

	return nil, "", 0
}

func (s *StorageServer) Run() error {
	// 获取 spacemesh plot file
	httpdaemon.RegisterRouter(httpdaemon.HttpRouter{
		Location: types.UploadPlotAPI,
		Handler:  s.UploadPlotRequest,
		Method:   http.MethodPost,
	})

	httpdaemon.Run(s.config.Port)
	return nil
}
