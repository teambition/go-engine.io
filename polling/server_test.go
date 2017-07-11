package polling

import (
	"bytes"
	"encoding/hex"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/teambition/go-engine.io/message"
	"github.com/teambition/go-engine.io/parser"
	"github.com/teambition/go-engine.io/transport"
)

func TestServer(t *testing.T) {
	Convey("Test polling", t, func() {

		Convey("Get", func() {
			f := newFakeCallback()
			w := httptest.NewRecorder()
			r, err := http.NewRequest("GET", "/", nil)
			So(err, ShouldBeNil)

			server, err := NewServer(w, r, f)
			So(err, ShouldBeNil)

			{
				writer, err := server.NextWriter(message.MessageBinary, parser.MESSAGE)
				So(err, ShouldBeNil)
				_, err = writer.Write([]byte("测试"))
				So(err, ShouldBeNil)
				err = writer.Close()
				So(err, ShouldBeNil)

				w := httptest.NewRecorder()
				r, err := http.NewRequest("GET", "/", nil)
				So(err, ShouldBeNil)

				server.ServeHTTP(w, r)

				So(w.Code, ShouldEqual, http.StatusOK)
				So(w.Header().Get("Content-Type"), ShouldEqual, "application/octet-stream")
				So(hex.EncodeToString(w.Body.Bytes()), ShouldEqual, "0107ff04e6b58be8af95")
				So(w.Body.String(), ShouldEqual, "\x01\x07\xff\x04测试")
			}

			{
				writer, err := server.NextWriter(message.MessageText, parser.MESSAGE)
				So(err, ShouldBeNil)
				_, err = writer.Write([]byte("测试"))
				So(err, ShouldBeNil)
				err = writer.Close()
				So(err, ShouldBeNil)

				w := httptest.NewRecorder()
				r, err := http.NewRequest("GET", "/", nil)
				So(err, ShouldBeNil)

				server.ServeHTTP(w, r)

				So(w.Code, ShouldEqual, http.StatusOK)
				So(w.Header().Get("Content-Type"), ShouldEqual, "application/octet-stream")
				So(hex.EncodeToString(w.Body.Bytes()), ShouldEqual, "0007ff34e6b58be8af95")
				So(w.Body.String(), ShouldEqual, "\x00\x07\xff4测试")
			}

			err = server.Close()
			So(err, ShouldBeNil)

			{
				w := httptest.NewRecorder()
				r, err := http.NewRequest("GET", "/", nil)
				So(err, ShouldBeNil)

				server.ServeHTTP(w, r)

				So(w.Code, ShouldEqual, http.StatusBadRequest)
				So(w.Body.String(), ShouldEqual, "closed\n")
			}

			{
				writer, err := server.NextWriter(message.MessageText, parser.MESSAGE)
				So(err, ShouldEqual, io.EOF)
				So(writer, ShouldBeNil)
			}
		})

		Convey("Get b64", func() {
			f := newFakeCallback()
			w := httptest.NewRecorder()
			r, err := http.NewRequest("GET", "/?b64", nil)
			So(err, ShouldBeNil)

			server, err := NewServer(w, r, f)
			So(err, ShouldBeNil)

			{
				writer, err := server.NextWriter(message.MessageBinary, parser.MESSAGE)
				So(err, ShouldBeNil)
				_, err = writer.Write([]byte("测试"))
				So(err, ShouldBeNil)
				err = writer.Close()
				So(err, ShouldBeNil)

				w := httptest.NewRecorder()
				r, err := http.NewRequest("GET", "/", nil)
				So(err, ShouldBeNil)

				server.ServeHTTP(w, r)

				So(w.Code, ShouldEqual, http.StatusOK)
				So(w.Header().Get("Content-Type"), ShouldEqual, "text/plain; charset=UTF-8")
				So(w.Body.String(), ShouldEqual, "10:b45rWL6K+V")
			}

			{
				writer, err := server.NextWriter(message.MessageText, parser.MESSAGE)
				So(err, ShouldBeNil)
				_, err = writer.Write([]byte("测试"))
				So(err, ShouldBeNil)
				err = writer.Close()
				So(err, ShouldBeNil)

				w := httptest.NewRecorder()
				r, err := http.NewRequest("GET", "/", nil)
				So(err, ShouldBeNil)

				server.ServeHTTP(w, r)

				So(w.Code, ShouldEqual, http.StatusOK)
				So(w.Header().Get("Content-Type"), ShouldEqual, "text/plain; charset=UTF-8")
				So(w.Body.String(), ShouldEqual, "7:4测试")
			}

			err = server.Close()
			So(err, ShouldBeNil)

			{
				w := httptest.NewRecorder()
				r, err := http.NewRequest("GET", "/", nil)
				So(err, ShouldBeNil)

				server.ServeHTTP(w, r)

				So(w.Code, ShouldEqual, http.StatusBadRequest)
				So(w.Body.String(), ShouldEqual, "closed\n")
			}

			{
				writer, err := server.NextWriter(message.MessageText, parser.MESSAGE)
				So(err, ShouldEqual, io.EOF)
				So(writer, ShouldBeNil)
			}
		})

		Convey("Post", func() {
			f := newFakeCallback()
			w := httptest.NewRecorder()
			r, err := http.NewRequest("GET", "/", nil)
			So(err, ShouldBeNil)

			server, err := NewServer(w, r, f)
			So(err, ShouldBeNil)

			go func() {
				<-f.onPacket
			}()

			{
				w := httptest.NewRecorder()
				r, err := http.NewRequest("POST", "/", bytes.NewBufferString("\x00\x07\xff4测试"))
				So(err, ShouldBeNil)

				server.ServeHTTP(w, r)

				So(w.Code, ShouldEqual, http.StatusOK)
				So(w.Body.String(), ShouldEqual, "ok")
				So(hex.EncodeToString(f.body), ShouldEqual, "e6b58be8af95")
			}

			{
				w := httptest.NewRecorder()
				r, err := http.NewRequest("POST", "/", bytes.NewBufferString("\x00\xff4测试"))
				So(err, ShouldBeNil)

				server.ServeHTTP(w, r)

				So(w.Code, ShouldEqual, http.StatusBadRequest)
				So(w.Body.String(), ShouldEqual, "invalid input\n")
			}

			err = server.Close()
			So(err, ShouldBeNil)

			{
				w := httptest.NewRecorder()
				r, err := http.NewRequest("POST", "/", bytes.NewBufferString("\x00\x07\xff4测试"))
				So(err, ShouldBeNil)

				server.ServeHTTP(w, r)

				So(w.Code, ShouldEqual, http.StatusBadRequest)
				So(w.Body.String(), ShouldEqual, "closed\n")
			}

		})

		Convey("Closing", func() {
			Convey("No get no post", func() {
				f := newFakeCallback()
				w := httptest.NewRecorder()
				r, err := http.NewRequest("GET", "/", nil)
				So(err, ShouldBeNil)

				server, err := NewServer(w, r, f)
				So(err, ShouldBeNil)

				So(f.ClosedCount(), ShouldEqual, 0)
				err = server.Close()
				So(err, ShouldBeNil)

				So(f.ClosedCount(), ShouldEqual, 1)
				So(f.closeServer, ShouldEqual, server)
			})

			Convey("No get has post", func() {
				f := newFakeCallback()
				sync := make(chan int)
				w := httptest.NewRecorder()
				r, err := http.NewRequest("GET", "/", nil)
				So(err, ShouldBeNil)

				server, err := NewServer(w, r, f)
				So(err, ShouldBeNil)

				go func() {
					w := httptest.NewRecorder()
					r, _ := http.NewRequest("POST", "/", bytes.NewBufferString("\x00\x07\xff4测试"))

					server.ServeHTTP(w, r)
					sync <- 1
				}()

				time.Sleep(time.Second)

				So(f.ClosedCount(), ShouldEqual, 0)
				err = server.Close()
				So(err, ShouldBeNil)
				So(f.ClosedCount(), ShouldEqual, 0)
				<-f.onPacket
				<-sync
				So(f.closeServer, ShouldEqual, server)
				So(f.ClosedCount(), ShouldEqual, 1)
			})

		})

	})
}
func TestHasGetAndPost(t *testing.T) {
	assert := assert.New(t)
	f := newFakeCallback()
	sync := make(chan int)
	w := httptest.NewRecorder()
	r, err := http.NewRequest("GET", "/", nil)
	assert.Nil(err)

	server, err := NewServer(w, r, f)
	assert.Nil(err)

	go func() {
		w := httptest.NewRecorder()
		r, _ := http.NewRequest("GET", "/", nil)

		server.ServeHTTP(w, r)

		sync <- 1
	}()

	go func() {
		w := httptest.NewRecorder()
		r, _ = http.NewRequest("POST", "/", bytes.NewBufferString("\x00\x07\xff4测试"))

		server.ServeHTTP(w, r)
		sync <- 1
	}()

	time.Sleep(100 * time.Millisecond)

	assert.Equal(0, f.ClosedCount())
	err = server.Close()
	assert.Nil(err)
	assert.Equal(0, f.ClosedCount())
	<-f.onPacket
	<-sync
	<-sync
	assert.Equal(server, f.closeServer)
	assert.Equal(1, f.ClosedCount())
}
func TestHasGet(t *testing.T) {
	assert := assert.New(t)
	f := newFakeCallback()
	sync := make(chan int)
	w := httptest.NewRecorder()
	r, err := http.NewRequest("GET", "/", nil)
	assert.Nil(err)

	server, err := NewServer(w, r, f)
	assert.Nil(err)

	go func() {
		w := httptest.NewRecorder()
		r, _ := http.NewRequest("GET", "/", nil)

		server.ServeHTTP(w, r)

		sync <- 1
	}()

	time.Sleep(time.Second)

	assert.Equal(0, f.ClosedCount())
	err = server.Close()
	assert.Nil(err)
	<-sync
	assert.Equal(server, f.closeServer)
	assert.Equal(1, f.ClosedCount())
}
func TestOverlayGet(t *testing.T) {
	assert := assert.New(t)
	sync := make(chan int)
	f := newFakeCallback()
	w := httptest.NewRecorder()
	r, err := http.NewRequest("GET", "/", nil)
	assert.Nil(err)

	server, err := NewServer(w, r, f)
	assert.Nil(err)

	go func() {
		w := httptest.NewRecorder()
		r, _ := http.NewRequest("GET", "/", nil)

		server.ServeHTTP(w, r)

		sync <- 1
	}()
	time.Sleep(100 * time.Millisecond)
	{
		w := httptest.NewRecorder()
		r, err := http.NewRequest("GET", "/", nil)
		assert.Nil(err)

		server.ServeHTTP(w, r)

		assert.Equal(http.StatusBadRequest, w.Code)
		assert.Equal("overlay get\n", w.Body.String())

	}
	server.Close()
	<-sync
	assert.Equal(1, f.ClosedCount())
}

func TestPost(t *testing.T) {
	assert := assert.New(t)

	sync := make(chan int)
	f := newFakeCallback()
	w := httptest.NewRecorder()
	r, err := http.NewRequest("GET", "/", nil)
	assert.Nil(err)

	server, err := NewServer(w, r, f)
	assert.Nil(err)

	go func() {
		w := httptest.NewRecorder()
		r, _ := http.NewRequest("POST", "/", bytes.NewBufferString("\x00\x07\xff4测试"))

		server.ServeHTTP(w, r)

		sync <- 1
	}()

	time.Sleep(100 * time.Millisecond)

	{
		w := httptest.NewRecorder()
		r, err := http.NewRequest("POST", "/", bytes.NewBufferString("\x00\x07\xff4测试"))
		assert.Nil(err)

		server.ServeHTTP(w, r)
		assert.Equal(http.StatusBadRequest, w.Code)
		assert.Equal("overlay post\n", w.Body.String())
	}

	<-f.onPacket
	server.Close()
	<-sync
	assert.Equal(1, f.ClosedCount())
}

func TestClosedBefore(t *testing.T) {
	assert := assert.New(t)
	f := newFakeCallback()
	w := httptest.NewRecorder()
	r, err := http.NewRequest("GET", "/", nil)
	assert.Nil(err)

	server, err := NewServer(w, r, f)
	assert.Nil(err)

	writer, err := server.NextWriter(message.MessageText, parser.MESSAGE)
	assert.Nil(err)

	for i := 0; i < 100; i++ {
		go server.Close()
	}
	time.Sleep(100 * time.Millisecond)
	err = writer.Close()
	assert.NotNil(err)
}

func TestMultiClose(t *testing.T) {
	assert := assert.New(t)
	f := newFakeCallback()
	sync := make(chan int)
	w := httptest.NewRecorder()
	r, err := http.NewRequest("GET", "/", nil)
	assert.Nil(err)

	server, err := NewServer(w, r, f)
	assert.Nil(err)

	go func() {
		w := httptest.NewRecorder()
		r, _ := http.NewRequest("GET", "/", nil)

		server.ServeHTTP(w, r)

		sync <- 1
	}()

	go func() {
		w := httptest.NewRecorder()
		r, _ := http.NewRequest("POST", "/", bytes.NewBufferString("\x00\x07\xff4测试"))

		server.ServeHTTP(w, r)
		sync <- 1
	}()

	time.Sleep(time.Second)

	assert.Equal(0, f.ClosedCount())
	err = server.Close()
	assert.Nil(err)
	assert.Equal(0, f.ClosedCount())
	<-f.onPacket
	<-sync
	<-sync
	assert.Equal(server, f.closeServer)
	for i := 1; i < 100; i++ {
		go server.Close()
	}
	time.Sleep(100 * time.Millisecond)
	assert.Equal(1, f.ClosedCount())
}

type fakeCallback struct {
	onPacket    chan bool
	messageType message.MessageType
	packetType  parser.PacketType
	body        []byte
	err         error
	closedCount int
	countLocker sync.Mutex
	closeServer transport.Server
}

func newFakeCallback() *fakeCallback {
	return &fakeCallback{
		onPacket: make(chan bool),
	}
}

func (f *fakeCallback) OnPacket(r *parser.PacketDecoder) {
	f.packetType = r.Type()
	f.messageType = r.MessageType()
	f.body, f.err = ioutil.ReadAll(r)
	f.onPacket <- true
}

func (f *fakeCallback) OnClose(s transport.Server) {
	f.countLocker.Lock()
	defer f.countLocker.Unlock()
	f.closedCount++
	f.closeServer = s
}

func (f *fakeCallback) ClosedCount() int {
	f.countLocker.Lock()
	defer f.countLocker.Unlock()
	return f.closedCount
}
