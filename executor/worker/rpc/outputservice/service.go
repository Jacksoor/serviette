package outputservice

type Service struct {
	format string
}

func New(format string) *Service {
	return &Service{
		format: format,
	}
}

func (s *Service) Format() string {
	return s.format
}

func (s *Service) SetFormat(req *struct {
	Format string `json:"format"`
}, resp *struct{}) error {
	s.format = req.Format
	return nil
}
