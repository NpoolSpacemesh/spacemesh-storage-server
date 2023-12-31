package mount

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	_ "sort"
	"strings"
	"sync"

	log "github.com/EntropyPool/entropy-logger"
	"github.com/NpoolSpacemesh/spacemesh-storage-server/util"
)

const (
	tmpFileSize = 101 * 1024 * 1024 * util.KB // 101G->kb
	mountRoot   = "/mnt"
	// TmpFileExt temporary file Extension
	TmpFileExt = ".tmp"

	mountPointMaxConcurrent = 1
)

type mountPointStatus struct {
	tasks uint8
}

func (m mountPointStatus) isIdle() bool {
	return m.tasks < mountPointMaxConcurrent
}

func (m *mountPointStatus) incTask() {
	if m.tasks >= 2 {
	} else {
		m.tasks = (m.tasks%mountPointMaxConcurrent + 1)
	}
}

func (m *mountPointStatus) desTask() {
	if m.tasks <= 0 {
	} else {
		m.tasks = (m.tasks - 1) % mountPointMaxConcurrent
	}
}

type (
	mountInfo struct {
		// 挂载点状态
		status mountPointStatus
		// 挂载点
		path string
		// 大小
		size uint64
	}

	// all mount point info
	mountInfos []mountInfo
)

var (
	_mountInfos   mountInfos
	lock          sync.Mutex
	curMountIndex int
	reservedSpace uint64
)

func (a mountInfos) Len() int      { return len(a) }
func (a mountInfos) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a mountInfos) Less(i, j int) bool {
	return a[i].path > a[j].path
}

// Choose the right moint point
func (a mountInfos) mount(dir string, diskSpace uint64) mountInfo {
	info := mountInfo{}
	index := 0

	for i := 0; i < len(a); i++ {
		_cur := (curMountIndex + i) % len(a)
		// current moint point working, continue
		if dir != "" {
			found := false
			filepath.Walk(a[_cur].path, func(path string, info os.FileInfo, err error) error {
				if strings.Contains(path, dir) {
					found = true
				}
				return nil
			})
			if !found {
				continue
			}
		} else {
			if !a[_cur].status.isIdle() {
				continue
			}
			if a[_cur].size < reservedSpace || a[_cur].size < diskSpace {
				log.Infof(log.Fields{}, "%v available %v < %v", a[_cur].path, a[_cur].size, reservedSpace)
				continue
			}
			// TODO isIdle incTask 可以包装在一起
			index = i
			a[_cur].status.incTask()
		}

		info = a[_cur]
		break
	}

	if 0 < len(a) {
		curMountIndex = (curMountIndex + index + 1) % len(a)
	}

	return info
}

// update moint point statue
func (a mountInfos) updateStatus(mountPoint string) {
	lock.Lock()
	for idx := range _mountInfos {
		if _mountInfos[idx].path == mountPoint {
			_mountInfos[idx].status.desTask()
			break
		}
	}
	lock.Unlock()
}

// Mount 寻找合适的目录
func Mount(dir string, diskSpace uint64) string {
	initMount()
	lock.Lock()
	path := _mountInfos.mount(dir, diskSpace).path
	if path == "" {
		path = _mountInfos.mount("", diskSpace).path
	}
	if path != "" {
		os.MkdirAll(filepath.Join(path, dir), 0666)
	}
	log.Infof(log.Fields{}, "select mount path %v, %v", path, dir)
	lock.Unlock()
	return path
}

// update mount point status
func SetMountPointIdle(mountPoint string) {
	_mountInfos.updateStatus(mountPoint)
}

// InitMount find all mount info
func InitMount(reserved uint64) error {
	reservedSpace = reserved
	tmps, _ := exec.Command("/usr/bin/find", "/mnt", "-name", "*.tmp").Output()
	tmpFiles := strings.Split(string(tmps), "\n")
	for _, tmp := range tmpFiles {
		log.Infof(log.Fields{}, "remove old tmps %v", tmp)
		exec.Command("/usr/bin/rm", "-rf", tmp).Run()
	}
	return initMount()
}

func initMount() error {
	mountPoints := make(map[string]mountInfo)
	filepath.Walk(mountRoot, func(absMountPath string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}

		// not a dir
		if !info.IsDir() {
			return nil
		}

		ok, err := util.IsMountPoint(absMountPath)
		if err != nil {
			return nil
		}

		if ok {
			avail, err := util.New(absMountPath).GetAvail()
			if err != nil {
				return nil
			}

			// 读取目录下的 tmp 文件个数, 每个累计101G
			mountPoints[absMountPath] = mountInfo{
				path: absMountPath,
				size: avail - getTmpFileSize(absMountPath),
			}
		}

		return nil
	})

	lock.Lock()
	mp := make(map[string]mountInfo)
	for _, v := range _mountInfos {
		mp[v.path] = v
	}

	// 防止掉盘,盘损坏
	_newmountInfos := []mountInfo{}
	for _, v := range mountPoints {
		if _, ok := mp[v.path]; ok {
			_newmountInfos = append(_newmountInfos, mountInfo{
				path:   v.path,
				size:   v.size,
				status: mp[v.path].status,
			})
		} else {
			_newmountInfos = append(_newmountInfos, v)
		}
		// log.Infof(log.Fields{}, "append valid mountpoint %v | %v", v.path, v.size)
	}
	_mountInfos = _newmountInfos
	// sort by mount point path
	sort.Sort(_mountInfos)
	lock.Unlock()

	return nil
}

// getTmpFileSize 获取指定目录下的 tmp 文件个数
func getTmpFileSize(root string) (size uint64) {
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}
		if filepath.Ext(path) == TmpFileExt {
			size += tmpFileSize
		}
		return nil
	})
	return
}
