package worker

type Supervisor struct {
	opts *WorkerOptions
}

func NewSupervisor(opts *WorkerOptions) *Supervisor {
	return &Supervisor{
		opts: opts,
	}
}

func (s *Supervisor) Spawn(arg0 string, argv []string, stdin []byte) *Worker {
	return newWorker(s.opts, arg0, argv, stdin)
}
