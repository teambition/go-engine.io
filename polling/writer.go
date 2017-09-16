package polling

import (
	"errors"
	"io"
)

func MakeSendChan() chan bool {
	return make(chan bool, 1)
}

type Writer struct {
	io.WriteCloser
	server *Polling
}

func NewWriter(w io.WriteCloser, server *Polling) *Writer {
	return &Writer{
		WriteCloser: w,
		server:      server,
	}
}
func (w *Writer) notify() (err error) {
	w.server.stateLocker.Lock()
	defer w.server.stateLocker.Unlock()
	if w.server.state != stateNormal {
		return errors.New("Error: use of closed network connection")
	}
	select {
	case w.server.sendChan <- true:
	default:
	}
	return
}

func (w *Writer) Close() (err error) {
	err = w.notify()
	if err != nil {
		return
	}
	return w.WriteCloser.Close()
}
