package scripts

import (
	"encoding/base64"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"

	"github.com/golang/protobuf/proto"

	scriptspb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
)

var (
	requirementsXattrName string = "user.kobun4.executor.requirements"
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
	rootPath      string
	accountHandle []byte
	name          string
}

func (s *Script) QualifiedName() string {
	return filepath.Join(base64.RawURLEncoding.EncodeToString(s.accountHandle), s.name)
}

func (s *Script) Path() string {
	return filepath.Join(s.rootPath, s.QualifiedName())
}

func (s *Script) AccountHandle() []byte {
	return s.accountHandle
}

func (s *Script) Name() string {
	return s.name
}

func (s *Script) Content() ([]byte, error) {
	return ioutil.ReadFile(s.Path())
}

func (s *Script) SetContent(content []byte) error {
	return ioutil.WriteFile(s.Path(), content, 0755)
}

func (s *Script) Requirements() (*scriptspb.Requirements, error) {
	reqs := &scriptspb.Requirements{}

	rawReqs, err := getxattr(s.Path(), requirementsXattrName)
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

func (s *Script) SetRequirements(reqs *scriptspb.Requirements) error {
	rawReqs, err := proto.Marshal(reqs)
	if err != nil {
		return err
	}
	return syscall.Setxattr(s.Path(), requirementsXattrName, rawReqs, 0)
}

func (s *Script) Delete() error {
	return os.Remove(s.Path())
}
