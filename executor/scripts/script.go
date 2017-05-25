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
	requestedCapabilitiesXattrName string = "user.kobun4.executor.capabilities.requested"
	accountCapabilitiesXattrPrefix        = "user.kobun4.executor.capabilities.account."
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

func (s *Script) Path() string {
	return filepath.Join(s.rootPath, base64.RawURLEncoding.EncodeToString(s.accountHandle), s.name)
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

func (s *Script) RequestedCapabilities() (*scriptspb.Capabilities, error) {
	caps := &scriptspb.Capabilities{}

	rawCaps, err := getxattr(s.Path(), requestedCapabilitiesXattrName)
	if err != nil {
		return nil, err
	}

	if rawCaps == nil {
		return caps, nil
	}

	if err := proto.Unmarshal(rawCaps, caps); err != nil {
		return nil, err
	}

	return caps, nil
}

func (s *Script) SetRequestedCapabilities(caps *scriptspb.Capabilities) error {
	rawCaps, err := proto.Marshal(caps)
	if err != nil {
		return err
	}
	return syscall.Setxattr(s.Path(), requestedCapabilitiesXattrName, rawCaps, 0)
}

func (s *Script) AccountCapabilities(accountHandle []byte) (*scriptspb.Capabilities, error) {
	caps := &scriptspb.Capabilities{}

	rawCaps, err := getxattr(s.Path(), accountCapabilitiesXattrPrefix+base64.RawURLEncoding.EncodeToString(accountHandle))
	if err != nil {
		return nil, err
	}

	if rawCaps == nil {
		return caps, nil
	}

	if err := proto.Unmarshal(rawCaps, caps); err != nil {
		return nil, err
	}

	return caps, nil
}

func (s *Script) SetAccountCapabilities(accountHandle []byte, caps *scriptspb.Capabilities) error {
	rawCaps, err := proto.Marshal(caps)
	if err != nil {
		return err
	}
	return syscall.Setxattr(s.Path(), accountCapabilitiesXattrPrefix+base64.RawURLEncoding.EncodeToString(accountHandle), rawCaps, 0)
}

func (s *Script) Delete() error {
	return os.Remove(s.Path())
}
