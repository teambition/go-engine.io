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
	"github.com/stretchr/testify/require"
	"github.com/teambition/go-engine.io/client"
	"github.com/teambition/go-engine.io/message"
	"github.com/teambition/go-engine.io/parser"
	"github.com/teambition/go-engine.io/polling"
	"github.com/teambition/go-engine.io/transport"
	"github.com/teambition/go-engine.io/websocket"
)

var (
	testData  = `{"jsonrpc": "2.0", "method": "subtract", "params": {"minuend": 42, "subtrahend": 23}, "id": 4}"`
	testBytes = []byte(`{"jsonrpc": "2.0", "method": "subtract", "params": {"minuend": 42, "subtrahend": 23}, "id": 4`)
)

func TestWebsocketMsg(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	req, err := http.NewRequest("GET", testWSURL+"?transport=websocket", nil)
	require.Nil(err)

	ws, err := client.NewWebsocket(req)
	assert.Nil(err)
	packet, err := ws.NextReader()
	require.Nil(err)

	assert.Equal(parser.OPEN, packet.Type())
	assert.Contains(readPacket(packet), `"upgrades":[],"pingInterval":25000,"pingTimeout":60000}`)
	packet.Close()

	w := sync.WaitGroup{}
	w.Add(1000)
	go func() {
		for i := 0; i < 1000; i++ {
			packet, err := ws.NextReader()
			require.Nil(err)
			assert.Equal(testData, readPacket(packet))
			packet.Close()
			w.Done()
		}
	}()
	for i := 0; i < 1000; i++ {
		err = writeMsgPacket(ws, testData)
		require.Nil(err)
	}
	w.Wait()
}

func TestPollingMsg(t *testing.T) {
	assert := assert.New(t)

	info := pollingHandShake(t)
	for i := 0; i < 1000; i++ {
		req, err := http.NewRequest("POST", testURL+"?transport=polling&sid="+info.Sid, nil)
		assert.Nil(err)
		assert.NotNil(req)
		polling, err := client.NewPolling(req)
		assert.Nil(err)
		err = writeMsgPacket(polling, testData)
		assert.Nil(err)

		req, err = http.NewRequest("GET", testURL+"?transport=polling&sid="+info.Sid, nil)
		assert.Nil(err)
		assert.NotNil(req)
		ws, err := client.NewPolling(req)
		assert.Nil(err)
		packet, err := ws.NextReader()
		assert.Nil(err)
		assert.Equal(testData, readPacket(packet))
		packet.Close()
	}
}

func TestCloseByWebsocket(t *testing.T) {
	assert := assert.New(t)

	server := newFakeServer()
	id := "id"
	var conn *serverConn
	lock := &sync.RWMutex{}

	h := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if conn == nil {
			var err error
			lock.Lock()
			conn, err = newServerConn(id, w, r, server)
			lock.Unlock()
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

	lock.RLock()
	if assert.NotNil(conn) {
		err = conn.Close()
		assert.Nil(err)
	}
	lock.RUnlock()
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

//func TestPollingMsg(t *testing.T) {
// 	assert := assert.New(t)
// 	require := require.New(t)

// 	req, err := http.NewRequest("GET", testURL+"?transport=polling", nil)
// 	require.Nil(err)
// 	assert.NotNil(req)
// 	ws, err := client.NewPolling(req)
// 	assert.Nil(err)
// 	packet, err := ws.NextReader()
// 	require.Nil(err)

// 	result := readPacket(packet)
// 	var info connectionInfo
// 	err = json.Unmarshal([]byte(result), &info)
// 	require.Nil(err)

// 	require.Contains(result, `"upgrades":["websocket"],"pingInterval":25000,"pingTimeout":60000}`)
// 	require.Equal(parser.OPEN, packet.Type())
// 	require.NotEmpty(info.Sid)

// 	packet.Close()

// 	w := sync.WaitGroup{}
// 	num := 1000
// 	w.Add(num)
// 	go func() {
// 		for i := 0; i < num; i++ {
// 			req, err := http.NewRequest("GET", testURL+"?transport=polling&sid="+info.Sid, nil)
// 			if err != nil {
// 				panic(err)
// 			}
// 			assert.NotNil(req)
// 			ws, err := client.NewPolling(req)
// 			if err != nil {
// 				panic(err)
// 			}
// 			packet, err := ws.NextReader()
// 			if err != nil {
// 				panic(err)
// 			}
// 			result := readPacket(packet)
// 			if result != "aa" {
// 				panic(result)
// 			}
// 			packet.Close()
// 			w.Done()
// 			log.Println("read:", i)
// 		}
// 	}()
// 	for i := 0; i < num; i++ {
// 		req, err := http.NewRequest("POST", testURL+"?transport=polling&sid="+info.Sid, nil)
// 		if err != nil {
// 			panic(err)
// 		}
// 		assert.NotNil(req)

// 		polling, err := client.NewPolling(req)
// 		assert.Nil(err)

// 		err = writeMsgPacket(polling, "aa")
// 		if err != nil {
// 			panic(err)
// 		}
// 		log.Println("client write:", i)
// 	}
// 	log.Println("write over")
// 	w.Wait()
// }
