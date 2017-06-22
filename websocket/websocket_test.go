package websocket

import (
	"encoding/hex"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/teambition/go-engine.io/client"
	"github.com/teambition/go-engine.io/message"
	"github.com/teambition/go-engine.io/parser"
	"github.com/teambition/go-engine.io/transport"
)

func TestServerPart(t *testing.T) {
	assert := assert.New(t)
	sync := make(chan int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f := newFakeCallback()
		s, err := NewServer(w, r, f)
		assert.Nil(err)
		defer s.Close()

		{
			w, err := s.NextWriter(message.MessageText, parser.OPEN)
			assert.Nil(err)
			err = w.Close()
			assert.Nil(err)
		}

		{
			<-f.onPacket
			assert.Equal(message.MessageBinary, f.messageType)
			assert.Equal(parser.MESSAGE, f.packetType)
			assert.Nil(err)
			assert.Equal(string(f.body), "测试")
		}

		<-sync
		sync <- 1

		<-sync
		sync <- 1

		{
			w, err := s.NextWriter(message.MessageBinary, parser.NOOP)
			assert.Nil(err)
			err = w.Close()
			assert.Nil(err)
		}

		<-sync
		sync <- 1

		{
			<-f.onPacket
			assert.Equal(message.MessageText, f.messageType)
			assert.Equal(parser.MESSAGE, f.packetType)
			assert.Nil(err)
			assert.Equal(hex.EncodeToString(f.body), "e697a5e69cace8aa9e")
		}

		<-sync
		sync <- 1
	}))
	defer server.Close()

	u, _ := url.Parse(server.URL)
	u.Scheme = "ws"
	req, err := http.NewRequest("GET", u.String(), nil)
	assert.Nil(err)
	c, _ := client.NewWebsocket(req)
	defer c.Close()
	{
		w, _ := c.NextWriter(message.MessageBinary, parser.MESSAGE)
		w.Write([]byte("测试"))
		w.Close()
	}

	sync <- 1
	<-sync

	{
		decoder, _ := c.NextReader()
		defer decoder.Close()
		ioutil.ReadAll(decoder)
	}

	sync <- 1
	<-sync

	{
		decoder, _ := c.NextReader()
		defer decoder.Close()
		ioutil.ReadAll(decoder)
	}

	sync <- 1
	<-sync

	{
		w, _ := c.NextWriter(message.MessageText, parser.MESSAGE)
		w.Write([]byte("日本語"))
		w.Close()
	}

	sync <- 1
	<-sync
}
func TestClientPart(t *testing.T) {
	assert := assert.New(t)
	sync := make(chan int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f := newFakeCallback()
		if v := r.URL.Query().Get("key"); v != "value" {
			t.Fatal(v, "!=", "value")
		}
		s, _ := NewServer(w, r, f)
		defer s.Close()

		{
			w, _ := s.NextWriter(message.MessageText, parser.OPEN)
			w.Close()
		}

		{
			<-f.onPacket
		}

		<-sync
		sync <- 1

		<-sync
		sync <- 1

		{
			w, _ := s.NextWriter(message.MessageBinary, parser.NOOP)
			w.Close()
		}

		<-sync
		sync <- 1

		{
			<-f.onPacket
		}

		<-sync
		sync <- 1
	}))
	defer server.Close()

	u, err := url.Parse(server.URL)
	assert.Nil(err)
	u.Scheme = "ws"
	req, err := http.NewRequest("GET", u.String()+"/?key=value", nil)
	assert.Nil(err)

	c, err := client.NewWebsocket(req)
	assert.Nil(err)
	defer c.Close()

	assert.NotNil(c.Response())
	assert.Equal(http.StatusSwitchingProtocols, c.Response().StatusCode)

	{
		w, err := c.NextWriter(message.MessageBinary, parser.MESSAGE)
		assert.Nil(err)
		_, err = w.Write([]byte("测试"))
		assert.Nil(err)
		err = w.Close()
		assert.Nil(err)
	}

	sync <- 1
	<-sync

	{
		decoder, err := c.NextReader()
		assert.Nil(err)
		defer decoder.Close()
		assert.Equal(message.MessageText, decoder.MessageType())
		assert.Equal(parser.OPEN, decoder.Type())
		b, err := ioutil.ReadAll(decoder)
		assert.Nil(err)
		assert.Equal(string(b), "")
	}

	sync <- 1
	<-sync

	{
		decoder, err := c.NextReader()
		assert.Nil(err)
		defer decoder.Close()
		assert.Equal(message.MessageBinary, decoder.MessageType())
		assert.Equal(parser.NOOP, decoder.Type())
		b, err := ioutil.ReadAll(decoder)
		assert.Nil(err)
		assert.Equal(string(b), "")
	}

	sync <- 1
	<-sync

	{
		w, err := c.NextWriter(message.MessageText, parser.MESSAGE)
		assert.Nil(err)
		_, err = w.Write([]byte("日本語"))
		assert.Nil(err)
		err = w.Close()
		assert.Nil(err)
	}

	sync <- 1
	<-sync
}
func TestPacketContent(t *testing.T) {
	assert := assert.New(t)
	sync := make(chan int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f := newFakeCallback()
		s, _ := NewServer(w, r, f)
		defer s.Close()

		{
			w, _ := s.NextWriter(message.MessageText, parser.MESSAGE)
			w.Write([]byte("日本語"))
			w.Close()
		}

		sync <- 1
		<-sync
	}))
	defer server.Close()

	u, err := url.Parse(server.URL)
	assert.Nil(err)
	u.Scheme = "ws"
	req, err := http.NewRequest("GET", u.String(), nil)
	assert.Nil(err)

	c, err := client.NewWebsocket(req)
	assert.Nil(err)
	defer c.Close()

	{
		client := c.(*client.Websocket)
		t, r, err := client.Conn.NextReader()
		assert.Nil(err)
		assert.Equal(websocket.TextMessage, t)
		b, err := ioutil.ReadAll(r)
		assert.Nil(err)
		assert.Equal("4日本語", string(b))
		assert.Equal("34e697a5e69cace8aa9e", hex.EncodeToString(b))
	}

	<-sync
	sync <- 1
}
func TestClose(t *testing.T) {
	assert := assert.New(t)
	f := newFakeCallback()
	var s transport.Server
	sync := make(chan int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s, _ = NewServer(w, r, f)
		s.Close()
		s.Close()
		s.Close()
		sync <- 1
	}))
	defer server.Close()

	u, err := url.Parse(server.URL)
	assert.Nil(err)
	u.Scheme = "ws"
	req, err := http.NewRequest("GET", u.String(), nil)
	assert.Nil(err)

	c, err := client.NewWebsocket(req)
	assert.Nil(err)
	defer c.Close()

	<-sync

	waitForClose(f)
	assert.Equal(1, f.ClosedCount())
	assert.Equal(s, f.closeServer)
}
func TestClosingByDisconnected(t *testing.T) {
	assert := assert.New(t)
	f := newFakeCallback()
	sync := make(chan int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s, _ := NewServer(w, r, f)
		server := s.(*Server)
		server.conn.Close()
		sync <- 1
	}))
	defer server.Close()

	u, err := url.Parse(server.URL)
	assert.Nil(err)
	u.Scheme = "ws"
	req, err := http.NewRequest("GET", u.String(), nil)
	assert.Nil(err)

	c, err := client.NewWebsocket(req)
	assert.Nil(err)
	defer c.Close()

	<-sync
	waitForClose(f)
	assert.Equal(1, f.ClosedCount())
}
func TestClosingWriterAfterClosed(t *testing.T) {
	f := newFakeCallback()
	sync := make(chan int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s, err := NewServer(w, r, f)
		if err != nil {
			t.Fatal(err)
		}
		writer, err := s.NextWriter(message.MessageText, parser.MESSAGE)
		if err != nil {
			t.Fatal(err)
		}
		err = s.Close()
		if err != nil {
			t.Fatal(err)
		}
		err = writer.Close()
		if err == nil {
			t.Fatal("err should not be nil")
		}
		sync <- 1
	}))
	defer server.Close()

	u, _ := url.Parse(server.URL)
	u.Scheme = "ws"
	req, _ := http.NewRequest("GET", u.String(), nil)

	c, _ := client.NewWebsocket(req)
	defer c.Close()

	<-sync
}

func waitForClose(f *fakeCallback) {
	timeout := time.After(5 * time.Second)

	var closed bool
	select {
	case <-f.closedChan:
		closed = true
	case <-timeout:
	}
	if closed != true {
		panic("waitForClose: closed != true")
	}
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
	closedChan  chan struct{}
}

func newFakeCallback() *fakeCallback {
	return &fakeCallback{
		onPacket:   make(chan bool),
		closedChan: make(chan struct{}),
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
	if f.closedCount == 1 {
		close(f.closedChan)
	}
}

func (f *fakeCallback) ClosedCount() int {
	f.countLocker.Lock()
	defer f.countLocker.Unlock()
	return f.closedCount
}
