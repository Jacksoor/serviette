package scripts

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"

	"github.com/golang/protobuf/proto"

	scriptspb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
)

var (
	metaXattrName string = "user.kobun4.executor"
)

func getxattr(path, name string) ([]byte, error) {
	size, err := syscall.Getxattr(path, name, nil)
	if err != nil {
		if err == syscall.ENODATA {
			return nil, nil
		}
		return nil, err
	}

	buf := make([]byte, size)
	read, err := syscall.Getxattr(path, name, buf)
	if err != nil {
		return nil, err
	}

	return buf[:read], nil
}

type Script struct {
	OwnerName string
	Name      string

	rootPath string
}

func (s *Script) QualifiedName() string {
	return filepath.Join(s.OwnerName, s.Name)
}

func (s *Script) Path() string {
	return filepath.Join(s.rootPath, s.QualifiedName())
}

func (s *Script) Content() ([]byte, error) {
	return ioutil.ReadFile(s.Path())
}

func (s *Script) SetContent(content []byte) error {
	return ioutil.WriteFile(s.Path(), content, 0755)
}

func (s *Script) Meta() (*scriptspb.Meta, error) {
	reqs := &scriptspb.Meta{}

	rawReqs, err := getxattr(s.Path(), metaXattrName)
	if err != nil {
		return nil, err
	}

	if rawReqs == nil {
		return reqs, nil
	}

	if err := proto.Unmarshal(rawReqs, reqs); err != nil {
		return nil, err
	}

	return reqs, nil
}

func (s *Script) SetMeta(reqs *scriptspb.Meta) error {
	rawReqs, err := proto.Marshal(reqs)
	if err != nil {
		return err
	}
	return syscall.Setxattr(s.Path(), metaXattrName, rawReqs, 0)
}

func (s *Script) Delete() error {
	return os.Remove(s.Path())
}
