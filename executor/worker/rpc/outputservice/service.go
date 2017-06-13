package outputservice

type Service struct {
	Format  string
	Private bool
}

func New(format string) *Service {
	return &Service{
		Format:  format,
		Private: false,
	}
}

func (s *Service) SetFormat(req *struct {
	Format string `json:"format"`
}, resp *struct{}) error {
	s.Format = req.Format
	return nil
}

func (s *Service) SetPrivate(req *struct {
	Private bool `json:"private"`
}, resp *struct{}) error {
	s.Private = req.Private
	return nil
}
