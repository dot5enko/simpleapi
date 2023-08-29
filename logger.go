package simpleapi

import "log"

type arrayLogger struct {
	lines []string
}

func (t *arrayLogger) Write(p []byte) (n int, err error) {
	if len(p) != 0 {
		t.lines = append(t.lines, string(p))
	}
	return len(p), nil
}

func new_debug_logger() (*log.Logger, *arrayLogger) {

	buf := &arrayLogger{}

	return log.New(buf, "", 0), buf

}
