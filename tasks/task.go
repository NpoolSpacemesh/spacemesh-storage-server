package tasks

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/NpoolSpacemesh/spacemesh-storage-server/pkg/mount"
	"github.com/NpoolSpacemesh/spacemesh-storage-server/util"
	"github.com/boltdb/bolt"
	"github.com/go-resty/resty/v2"
	"golang.org/x/xerrors"

	log "github.com/EntropyPool/entropy-logger"
	httpdaemon "github.com/NpoolRD/http-daemon"
	spacemeshstorageProxyTypes "github.com/NpoolSpacemesh/spacemesh-storage-server/proxytypes"
)

func Fetch(input Meta) {
	var (
		done   bool  = false
		status uint8 = TaskFinish
		resp   *resty.Response
	)

	defer func() {
		if !done {
			update(input.PlotURL, TaskFail)
		}
	}()

	plotFile := filepath.Base(input.PlotURL)
	plotDir := filepath.Base(filepath.Dir(input.PlotURL))

	path := mount.Mount(plotDir, input.DiskSpace)
	// 没有挂载的盘符
	if path == "" {
		// TODO
		err := xerrors.Errorf("no suitable path found")
		log.Errorf(log.Fields{}, "fail to select disk for %v: %v", input.PlotURL, err)
		return
	}

	// down task
	defer mount.SetMountPointIdle(path)

	// 选择存放的目录
	log.Infof(log.Fields{}, "try to select suitable path %v for %v", path, input.PlotURL)

	tmp := filepath.Join(temp(path, input.ClusterName, filepath.Join(plotDir, plotFile), true)...)
	os.MkdirAll(filepath.Dir(tmp), 0666)

	plot, err := os.Create(tmp)
	if err != nil {
		log.Errorf(log.Fields{}, "fail to create tmp for %v: %v", input.PlotURL, err)
		return
	}
	defer plot.Close()

	// 移除临时文件
	defer func() {
		// 文件存在
		_, err := os.Stat(tmp)
		if err == nil {
			os.Remove(tmp)
		}
	}()

	resp, err = httpdaemon.R().SetDoNotParseResponse(true).Get(input.PlotURL)
	if err != nil {
		log.Errorf(log.Fields{}, "fail to get file content for %v: %v", input.PlotURL, err)
		return
	}

	defer resp.RawBody().Close()
	if resp.StatusCode() != http.StatusOK {
		log.Errorf(log.Fields{}, "get file content for %v resp status code %v not %v", input.PlotURL, resp.StatusCode(), http.StatusOK)
		return
	}

	if _, err = io.Copy(plot, resp.RawBody()); err != nil {
		log.Errorf(log.Fields{}, "fail to write file content for %v: %v", input.PlotURL, err)
		return
	}

	plotFile = filepath.Join(temp(path, input.ClusterName, filepath.Join(plotDir, plotFile), false)...)
	os.MkdirAll(filepath.Dir(plotFile), 0666)
	if err = os.Rename(tmp, plotFile); err != nil {
		log.Errorf(log.Fields{}, "fail to rename tmp file for %v: %v", input.PlotURL, err)
		return
	}

	// update bolt database
	err = update(input.PlotURL, status)
	done = true
	return
}

func Finsih(input Meta) {
	log.Infof(log.Fields{}, "finish %v", input.PlotURL)
	// notify client write plot file result
	finish := spacemeshstorageProxyTypes.FinishPlotInput{
		PlotFile: input.PlotURL,
	}
	body, _ := json.Marshal(finish)
	resp, err := httpdaemon.R().
		SetHeader("Content-Type", "application/json").
		SetBody(body).
		Post(input.FinishURL)
	if err != nil {
		return
	}
	// TODO
	if resp.StatusCode() != http.StatusOK {
	}
	update(input.PlotURL, TaskDone)
}

func Fail(input Meta) {
	log.Infof(log.Fields{}, "fail %v", input.PlotURL)
	fail := spacemeshstorageProxyTypes.FailPlotInput{
		PlotFile: input.PlotURL,
	}
	body, _ := json.Marshal(fail)
	resp, err := httpdaemon.R().
		SetHeader("Content-Type", "application/json").
		SetBody(body).
		Post(input.FailURL)
	if err != nil {
		return
	}
	// TODO
	if resp.StatusCode() != http.StatusOK {
	}
	update(input.PlotURL, TaskDone)
}

func update(key string, status uint8) error {
	db, err := util.BoltClient()
	if err != nil {
		return err
	}

	return db.Update(func(tx *bolt.Tx) error {
		bk := tx.Bucket(util.DefaultBucket)
		r := bk.Get([]byte(key))
		if r == nil {
			return errors.New("bolt key not exist")
		}

		// 删除原有的 key
		if err := bk.Delete([]byte(key)); err != nil {
			return err
		}

		meta := Meta{}
		if err := json.Unmarshal(r, &meta); err != nil {
			return err
		}
		meta.Status = status
		_meta, err := json.Marshal(meta)
		if err != nil {
			return err
		}

		// 插入最新的 key
		return bk.Put([]byte(meta.PlotURL), _meta)
	})
}

func temp(mountPoint, clusterName, src string, temp bool) []string {
	// [1] mnt [2] sda
	_paths := strings.Split(mountPoint, "/")
	subPath := strings.TrimRightFunc(_paths[2][2:], func(r rune) bool {
		if unicode.IsDigit(r) {
			return true
		}
		return false
	})
	if temp {
		return []string{
			mountPoint,
			fmt.Sprintf("gv%s", subPath),
			clusterName,
			src + mount.TmpFileExt,
		}
	}
	return []string{
		mountPoint,
		fmt.Sprintf("gv%s", subPath),
		clusterName,
		src,
	}
}
