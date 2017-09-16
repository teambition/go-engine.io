package polling

import (
	"errors"
	"io"
	"time"
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

const writeTimeout = 3 * time.Second

func (w *Writer) notify() (err error) {
	w.server.stateLocker.RLock()
	defer w.server.stateLocker.RUnlock()
	if w.server.state != stateNormal {
		return errors.New("Error: use of closed network connection")
	}
	select {
	case w.server.sendChan <- true:
	case <-time.After(writeTimeout):
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
