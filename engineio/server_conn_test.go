package engineio

import (
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/teambition/go-engine.io/client"
	"github.com/teambition/go-engine.io/message"
	"github.com/teambition/go-engine.io/parser"
	"github.com/teambition/go-engine.io/polling"
	"github.com/teambition/go-engine.io/transport"
	"github.com/teambition/go-engine.io/websocket"
)

func TestWithWebsocket(t *testing.T) {
	assert := assert.New(t)
	server := newFakeServer()
	h := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := newServerConn("id", w, r, server)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()

		if conn.Id() != "id" {
			t.Fatal(err)
		}
		if conn.Request() != r {
			t.Fatal(err)
		}
	}))
	defer h.Close()

	u, _ := url.Parse(h.URL)
	u.Scheme = "ws"
	req, err := http.NewRequest("GET", u.String()+"/?transport=websocket", nil)
	assert.Nil(err)
	assert.NotNil(req)

	c, err := client.NewWebsocket(req)
	assert.Nil(err)
	time.Sleep(100 * time.Millisecond)
	defer c.Close()
}
func TestCloseByWebsocket(t *testing.T) {
	assert := assert.New(t)

	server := newFakeServer()
	id := "id"
	var conn *serverConn

	h := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if conn == nil {
			var err error
			conn, err = newServerConn(id, w, r, server)
			if err != nil {
				log.Println(err)
			}
		}
		conn.ServeHTTP(w, r)
	}))
	defer h.Close()

	u, err := url.Parse(h.URL)
	assert.Nil(err)

	u.Scheme = "ws"
	req, err := http.NewRequest("GET", u.String()+"/?transport=websocket", nil)
	assert.Nil(err)
	wc, err := client.NewWebsocket(req)
	assert.Nil(err)

	time.Sleep(100 * time.Millisecond)
	wc.Close()
	time.Sleep(100 * time.Millisecond)
	assert.Equal(1, server.getClosedVal(id))

	if assert.NotNil(conn) {
		err = conn.Close()
		assert.Nil(err)
	}
}
func TestWithoutTransport(t *testing.T) {
	assert := assert.New(t)
	server := newFakeServer()
	req, err := http.NewRequest("GET", "/", nil)
	assert.Nil(err)
	resp := httptest.NewRecorder()
	_, err = newServerConn("id", resp, req, server)
	assert.Equal(InvalidError, err)
}

func TestInvalidTransport(t *testing.T) {
	assert := assert.New(t)
	server := newFakeServer()
	req, err := http.NewRequest("GET", "/?transport=websocket", nil)
	assert.Nil(err)
	resp := httptest.NewRecorder()
	_, err = newServerConn("id", resp, req, server)
	assert.NotNil(err)
}
func TestWithPolling(t *testing.T) {
	assert := assert.New(t)
	server := newFakeServer()
	req, err := http.NewRequest("GET", "/?transport=polling", nil)
	assert.Nil(err)
	resp := httptest.NewRecorder()
	conn, err := newServerConn("id", resp, req, server)
	assert.Nil(err)
	assert.Equal("id", conn.Id())
	assert.Equal(req, conn.Request())
	conn.Close()
}
func TestCloseUpgrading(t *testing.T) {
	assert := assert.New(t)
	server := newFakeServer()
	id := "id"
	var conn *serverConn

	h := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if conn == nil {
			var err error
			conn, err = newServerConn(id, w, r, server)
			if err != nil {
				t.Fatal(err)
			}
		}

		conn.ServeHTTP(w, r)
	}))
	defer h.Close()

	u, err := url.Parse(h.URL)
	assert.Nil(err)

	req, err := http.NewRequest("GET", u.String()+"/?transport=polling", nil)
	assert.Nil(err)
	pc, err := client.NewPolling(req)
	assert.Nil(err)

	decoder, err := pc.NextReader()
	assert.Nil(err)
	assert.Equal(http.StatusOK, pc.Response().StatusCode)

	assert.NotNil(conn)
	assert.Equal(message.MessageText, decoder.MessageType())
	assert.Equal(parser.OPEN, decoder.Type())

	assert.NotNil(conn.getCurrent())
	assert.Nil(conn.getUpgrade())

	u.Scheme = "ws"
	req, err = http.NewRequest("GET", u.String()+"/?transport=websocket", nil)
	assert.Nil(err)
	wc, err := client.NewWebsocket(req)
	assert.Nil(err)

	time.Sleep(10 * time.Millisecond)
	assert.NotNil(conn.getCurrent())
	assert.NotNil(conn.getUpgrade())

	encoder, err := wc.NextWriter(message.MessageBinary, parser.PING)
	assert.Nil(err)
	encoder.Write([]byte("probe"))
	encoder.Close()

	decoder, err = wc.NextReader()
	assert.Nil(err)
	assert.Equal(http.StatusSwitchingProtocols, wc.Response().StatusCode)

	err = conn.Close()
	assert.Nil(err)

	wc.Close()
	pc.Close()
	time.Sleep(time.Second)

	assert.Equal(1, server.getClosedVal(id))
}

func TestTimeoutByPolling(t *testing.T) {
	assert := assert.New(t)
	server := newFakeServer()
	id := "id"
	var conn *serverConn

	h := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if conn == nil {
			var err error
			conn, err = newServerConn(id, w, r, server)
			assert.Nil(err)
		}
		conn.ServeHTTP(w, r)
	}))
	defer h.Close()

	u, err := url.Parse(h.URL)
	assert.Nil(err)

	req, err := http.NewRequest("GET", u.String()+"/?transport=polling", nil)
	assert.Nil(err)

	client, err := client.NewPolling(req)
	assert.Nil(err)

	decoder, err := client.NextReader()
	assert.Nil(err)

	assert.Equal(message.MessageText, decoder.MessageType())
	assert.Equal(parser.OPEN, decoder.Type())

	client.Close()

	err = conn.Close()
	assert.Nil(err)

	assert.Equal(1, server.getClosedVal(id))
}
func TestPollingToWebsocket(t *testing.T) {
	assert := assert.New(t)
	server := newFakeServer()
	id := "id"
	var conn *serverConn

	h := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if conn == nil {
			var err error
			conn, err = newServerConn(id, w, r, server)
			assert.Nil(err)
		}
		conn.ServeHTTP(w, r)
	}))
	defer h.Close()

	u, err := url.Parse(h.URL)
	assert.Nil(err)
	req, err := http.NewRequest("GET", u.String()+"/?transport=polling", nil)
	assert.Nil(err)
	pollingClient, err := client.NewPolling(req)
	assert.Nil(err)

	decoder, err := pollingClient.NextReader()
	assert.Nil(err)
	assert.Equal(http.StatusOK, pollingClient.Response().StatusCode)
	assert.NotNil(conn)
	assert.Equal(message.MessageText, decoder.MessageType())
	assert.Equal(parser.OPEN, decoder.Type())
	assert.NotNil(conn.getCurrent())
	assert.Nil(conn.getUpgrade())

	u.Scheme = "ws"
	req, err = http.NewRequest("GET", u.String()+"/?transport=websocket", nil)
	assert.Nil(err)
	wc, err := client.NewWebsocket(req)
	assert.Nil(err)
	time.Sleep(50 * time.Millisecond)
	assert.NotNil(conn.getCurrent())
	assert.NotNil(conn.getUpgrade())

	encoder, err := wc.NextWriter(message.MessageBinary, parser.PING)
	assert.Nil(err)
	encoder.Write([]byte("probe"))
	encoder.Close()

	decoder, err = wc.NextReader()
	assert.Nil(err)
	assert.Equal(http.StatusSwitchingProtocols, wc.Response().StatusCode)
	assert.Equal(message.MessageText, decoder.MessageType())
	assert.Equal(parser.PONG, decoder.Type())

	pollingClient.Close()

	encoder, err = wc.NextWriter(message.MessageBinary, parser.UPGRADE)
	assert.Nil(err)
	encoder.Close()

	err = conn.Close()
	time.Sleep(10 * time.Millisecond)
	assert.Nil(err)
	assert.Equal(1, server.getClosedVal(id))
	wc.Close()
}

type FakeServer struct {
	config       *config
	creaters     transportCreaters
	closed       map[string]int
	closedLocker sync.Mutex
}

func newFakeServer() *FakeServer {
	return &FakeServer{
		config: &config{
			PingTimeout:   time.Second * 2,
			PingInterval:  time.Second * 1,
			AllowUpgrades: true,
		},
		creaters: transportCreaters{
			"polling": transport.Creater{
				Name:   "polling",
				Server: polling.NewServer,
			},
			"websocket": transport.Creater{
				Name:   "websocket",
				Server: websocket.NewServer,
			},
		},
		closed: make(map[string]int),
	}
}

func (f *FakeServer) configure() config {
	return *f.config
}

func (f *FakeServer) transports() transportCreaters {
	return f.creaters
}

func (f *FakeServer) onClose(sid string) {
	f.closedLocker.Lock()
	defer f.closedLocker.Unlock()
	f.closed[sid] = f.closed[sid] + 1
}
func (f *FakeServer) getClosedVal(sid string) int {
	f.closedLocker.Lock()
	defer f.closedLocker.Unlock()
	return f.closed[sid]
}
