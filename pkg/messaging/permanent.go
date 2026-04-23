package messaging

import "errors"

type permanentErr struct {
	err error
}

func (e *permanentErr) Error() string { return e.err.Error() }
func (e *permanentErr) Unwrap() error { return e.err }

func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return &permanentErr{err: err}
}

func IsPermanent(err error) bool {
	var p *permanentErr
	return errors.As(err, &p)
}
