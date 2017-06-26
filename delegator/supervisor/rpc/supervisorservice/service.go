package supervisorservice

import (
	"os/exec"
)

type Service struct {
	processes map[uint32]*exec.Cmd
}

func New() *Service {
	return &Service{
		processes: make(map[uint32]*exec.Cmd, 0),
	}
}
