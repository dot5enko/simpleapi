package simpleapi

import "fmt"

type RespErr struct {
	Data     HM
	Httpcode int
}

func (r RespErr) Error() string {
	return fmt.Sprintf("RespErr : %d", r.Httpcode)
}

func NewRespErr(code int, d HM) *RespErr {
	return &RespErr{
		Data:     d,
		Httpcode: code,
	}
}
