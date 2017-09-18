package polling

import (
	"bytes"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/teambition/go-engine.io/apperrs"
)

func TestWriter(t *testing.T) {
	assert := assert.New(t)
	p := &Polling{
		state:    stateNormal,
		sendChan: MakeSendChan(),
	}
	sendChan := p.sendChan

	Convey("Wait close", t, func() {
		w := newFakeWriteCloser()

		select {
		case <-sendChan:
			panic("should not run here")
		default:
		}

		writer := NewWriter(w, p)
		err := writer.Close()
		So(err, ShouldBeNil)

		select {
		case <-sendChan:
		default:
			panic("should not run here")
		}

		select {
		case <-sendChan:
			panic("should not run here")
		default:
		}
	})

	Convey("Many writer with close", t, func() {
		for i := 0; i < 10; i++ {
			w := newFakeWriteCloser()
			writer := NewWriter(w, p)
			err := writer.Close()
			if i == 0 {
				assert.Nil(err)
			} else {
				assert.Equal(apperrs.ErrPollingRequestNotFound, err)
			}
		}

		select {
		case <-sendChan:
		default:
			panic("should not run here")
		}

		select {
		case <-sendChan:
			panic("should not run here")
		default:
		}
	})

	Convey("Close with not normal", t, func() {
		p := &Polling{
			state:    stateClosing,
			sendChan: MakeSendChan(),
		}

		w := newFakeWriteCloser()
		writer := NewWriter(w, p)
		err := writer.Close()
		So(err, ShouldNotBeNil)
	})
}

type fakeWriteCloser struct {
	*bytes.Buffer
}

func newFakeWriteCloser() *fakeWriteCloser {
	return &fakeWriteCloser{
		Buffer: bytes.NewBuffer(nil),
	}
}

func (f *fakeWriteCloser) Close() error {
	return nil
}
