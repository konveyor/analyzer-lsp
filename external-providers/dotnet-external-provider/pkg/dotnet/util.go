package dotnet

import "github.com/go-logr/logr"

type received struct {
	l logr.Logger
}

func (r *received) Write(p []byte) (int, error) {
	n := len(p)
	r.l.Info(string(p))
	return n, nil
}

type sent struct {
	l logr.Logger
}

func (s *sent) Write(p []byte) (int, error) {
	n := len(p)
	s.l.Info(string(p))
	return n, nil
}
