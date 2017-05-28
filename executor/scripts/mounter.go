package scripts

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/golang/glog"
)

type Mounter struct {
	imagesRoot string
	imageSize  int64

	mountsRoot string

	mu sync.Mutex
}

func NewMounter(imagesRoot string, imageSize int64) (*Mounter, error) {
	mountsRoot, err := ioutil.TempDir("", "kobun4-mounts-")
	if err != nil {
		return nil, err
	}

	return &Mounter{
		imagesRoot: imagesRoot,
		imageSize:  imageSize,

		mountsRoot: mountsRoot,
	}, nil
}

func (m* Mounter) MountsRoot() string {
	return m.mountsRoot
}

func (m *Mounter) Mount(scriptAccountHandle []byte) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	encodedHandle := base64.RawURLEncoding.EncodeToString(scriptAccountHandle)

	mountPath := filepath.Join(m.mountsRoot, encodedHandle)
	if err := os.Mkdir(mountPath, 0700); err != nil {
		if !os.IsExist(err) {
			return "", err
		}
		return mountPath, nil
	}

	imagePath := filepath.Join(m.imagesRoot, encodedHandle)
	if _, err := os.Stat(imagePath); err != nil {
		if !os.IsNotExist(err) {
			return "", err
		}

		f, err := os.Create(imagePath)
		if err != nil {
			return "", err
		}

		if err := f.Truncate(m.imageSize); err != nil {
			f.Close()
			return "", err
		}
		f.Close()

		if err := exec.Command("mkfs.ntfs", "-F", imagePath).Run(); err != nil {
			if eErr, ok := err.(*exec.ExitError); ok {
				return "", fmt.Errorf("mkfs.ntfs %v: %v", eErr, string(eErr.Stderr))
			}
			return "", fmt.Errorf("mkfs.ntfs: %v", err)
		}
	}

	if err := exec.Command("ntfs-3g", imagePath, mountPath).Run(); err != nil {
		if err := os.Remove(mountPath); err != nil {
			panic(err)
		}
		if eErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("ntfs-3g %v: %v", eErr, string(eErr.Stderr))
		}
		return "", fmt.Errorf("ntfs-3g: %v", err)
	}

	return mountPath, nil
}

func (m *Mounter) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	files, err := ioutil.ReadDir(m.mountsRoot)
	if err != nil {
		return err
	}

	for _, file := range files {
		mountPoint := filepath.Join(m.mountsRoot, file.Name())
		if err := exec.Command("fusermount", "-u", mountPoint).Run(); err != nil {
			return err
		}

		if err := os.Remove(mountPoint); err != nil {
			glog.Errorf("Failed to remove mount point, ignoring: %v", err)
		}
	}

	if err := os.RemoveAll(m.mountsRoot); err != nil {
		return err
	}

	return nil
}
