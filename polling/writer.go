package polling

import (
	"io"

	"github.com/teambition/go-engine.io/apperrs"
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
func (w *Writer) notify() error {
	w.server.stateLocker.Lock()
	defer w.server.stateLocker.Unlock()
	if w.server.state != stateNormal {
		return apperrs.ErrPollingConnectionClosed
	}
	select {
	case w.server.sendChan <- true:
		return nil
	default:
		return apperrs.ErrPollingRequestNotFound
	}
}

func (w *Writer) Close() (err error) {
	err = w.WriteCloser.Close()
	if err != nil {
		return
	}
	return w.notify()
}
