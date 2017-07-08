package engineio

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	wsclient "github.com/mushroomsir/engine.io"
	"github.com/mushroomsir/engine.io/transports"
	"github.com/stretchr/testify/assert"
	"github.com/teambition/go-engine.io/parser"
)

func TestSetupServer(t *testing.T) {
	assert := assert.New(t)
	server, err := NewServer(nil)
	assert.Nil(err)

	server.SetPingInterval(time.Second)
	assert.Equal(time.Second, server.config.PingInterval)

	server.SetPingTimeout(10 * time.Second)
	assert.Equal(10*time.Second, server.config.PingTimeout)

	f := func(*http.Request) error { return nil }
	server.SetAllowRequest(f)
	assert.Nil(server.config.AllowRequest(nil))

	server.SetAllowUpgrades(false)
	assert.False(server.config.AllowUpgrades)

	server.SetCookie("prefix")
	assert.Equal("prefix", server.config.Cookie)

}
func TestCreateServer(t *testing.T) {
	assert := assert.New(t)

	req, err := http.NewRequest("GET", "/", nil)
	assert.Nil(err)

	id1 := newId(req)
	time.Sleep(time.Millisecond)
	id2 := newId(req)
	assert.NotEqual(id1, id2)

	server, err := NewServer([]string{"xwebsocket"})
	assert.Nil(server)
	assert.NotNil(err)

	server, _ = NewServer(nil)
	assert.Nil(server.config.AllowRequest(nil))
	assert.Equal(0, server.Count())

	server.SetNewId(newId)
	assert.NotNil(server.configure())
	assert.NotNil(server.transports())

	server.SetSessionManager(nil)
}
func TestWebsocket(t *testing.T) {
	assert := assert.New(t)
	sync := make(chan int)
	var wsurl string

	testData1 := []byte(`{"jsonrpc": "2.0", "method": "subtract", "params": {"minuend": 42, "subtrahend": 23}, "id": 4`)
	testData1Reply := []byte(`{"jsonrpc": "2.0", "result": 19, "id": 4}`)
	go func() {
		websocket, _ := NewServer(nil)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			websocket.ServeHTTP(w, r)
		}))
		wsurl = getURL(server.URL)
		sync <- 1
		conn, err := websocket.Accept()
		assert.Nil(err)
		for i := 0; i < 10; i++ {
			b, err := read(conn)
			assert.Nil(err)
			assert.Equal(testData1, b)
			write(conn, testData1Reply)
		}
		b, err := read(conn)
		assert.Nil(b)
		assert.Equal(io.EOF, err)
		sync <- 1
	}()
	<-sync
	client, _ := wsclient.NewClient(wsurl)

	event := <-client.Event
	assert.Equal("open", event.Type)
	assert.Contains(string(event.Data), `"upgrades":["polling"],"pingInterval":25000,"pingTimeout":60000}`)
	assert.NotNil(event.Data)
	for i := 0; i < 10; i++ {
		client.SendMessage(testData1)

		event := <-client.Event
		assert.Equal("message", event.Type)
		assert.Equal(testData1Reply, event.Data)
	}
	err := client.Close()
	assert.Nil(err)
	<-sync
}
func TestPolling(t *testing.T) {
	assert := assert.New(t)
	sync := make(chan int)
	var wsurl, sid string

	testData1 := []byte(`{"jsonrpc": "2.0", "method": "subtract", "params": {"minuend": 42, "subtrahend": 23}, "id": 4`)
	testData1Reply := []byte(`47:4{"jsonrpc": "2.0", "result": "OK", "id": 4}`)
	go func() {
		websocket, _ := NewServer(nil)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			websocket.ServeHTTP(w, r)
		}))
		wsurl = server.URL
		sync <- 1
		conn, err := websocket.Accept()

		assert.Nil(err)
		sid = conn.Id()
		assert.NotEmpty(sid)

		err = write(conn, testData1)
		assert.Nil(err)

		b, err := read(conn)
		assert.Nil(err)
		assert.Equal(`{"jsonrpc": "2.0", "result": "OK", "id": 4}`, string(b))
		sync <- 1
	}()
	<-sync
	pollingClient := &http.Client{}
	res, err := pollingClient.Do(newOpenReq(wsurl))
	assert.Nil(err)
	assert.Equal(200, res.StatusCode)
	bodyBytes, err := ioutil.ReadAll(res.Body)
	assert.Contains(string(bodyBytes), `"upgrades":["websocket"],"pingInterval":25000,"pingTimeout":60000}`)

	res, err = pollingClient.Do(newOpenReq(wsurl + "?sid=" + sid))
	bodyBytes, err = ioutil.ReadAll(res.Body)
	assert.Contains(string(bodyBytes), string(testData1))

	res, err = pollingClient.Post(wsurl+"?transport=polling&sid="+sid, "application/json", bytes.NewBuffer(testData1Reply))
	assert.Nil(err)
	assert.Equal(200, res.StatusCode)
	bodyBytes, err = ioutil.ReadAll(res.Body)
	assert.Equal(string(bodyBytes), "ok")
	<-sync

}
func TestPing(t *testing.T) {
	assert := assert.New(t)
	sync := make(chan int)
	var wsurl string

	go func() {
		websocket, _ := NewServer(nil)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			websocket.ServeHTTP(w, r)
		}))
		wsurl = getURL(server.URL)
		sync <- 1
		conn, err := websocket.Accept()
		assert.Nil(err)

		<-sync
		ping1 := conn.LastPing()
		<-sync
		ping2 := conn.LastPing()
		a1 := ping1.Equal(ping2)
		assert.False(a1)

		a2 := ping2.After(ping1)
		assert.True(a2)
		sync <- 1
	}()
	<-sync
	client, _ := wsclient.NewClient(wsurl)

	event := <-client.Event
	assert.Equal("open", event.Type)
	assert.Contains(string(event.Data), `"upgrades":["polling"],"pingInterval":25000,"pingTimeout":60000}`)

	client.SendPacket(&transports.Packet{Type: transports.Ping})
	sync <- 1
	time.Sleep(20 * time.Millisecond)
	client.SendPacket(&transports.Packet{Type: transports.Ping})
	sync <- 1
	<-sync

}
func TestUpgrades(t *testing.T) {
	assert := assert.New(t)
	sync := make(chan int)
	var wsurl, sid string

	go func() {
		websocket, _ := NewServer(nil)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			websocket.ServeHTTP(w, r)
		}))
		wsurl = server.URL
		sync <- 1
		conn, err := websocket.Accept()
		assert.Nil(err)
		sid = conn.Id()
		<-sync
	}()
	<-sync

	pollingClient := &http.Client{}
	res, err := pollingClient.Do(newOpenReq(wsurl))

	assert.Nil(err)
	assert.Equal(200, res.StatusCode)
	bodyBytes, err := ioutil.ReadAll(res.Body)
	assert.Contains(string(bodyBytes), `"upgrades":["websocket"],"pingInterval":25000,"pingTimeout":60000}`)

	go func() {
		time.Sleep(10 * time.Millisecond)
		client, err := wsclient.NewClient(getURL(wsurl) + "?transport=websocket&sid=" + sid)
		if assert.Nil(err) {
			client.SendPacket(&transports.Packet{Type: transports.Ping, Data: []byte("probe")})
		}
		event := <-client.Event
		assert.Equal("pong", event.Type)
		assert.Equal("probe", string(event.Data))

		client.SendPacket(&transports.Packet{Type: transports.Upgrade})
		time.Sleep(20 * time.Millisecond)
	}()
	res, err = pollingClient.Do(newOpenReq(wsurl + "?sid=" + sid))
	assert.Nil(err)
	assert.Equal(200, res.StatusCode)
	bodyBytes, err = ioutil.ReadAll(res.Body)
	assert.Contains(string(bodyBytes), "6")
	time.Sleep(20 * time.Millisecond)

}
func getURL(addr string) string {
	u, _ := url.Parse(addr)
	u.Scheme = "ws"
	return u.String()
}
func newOpenReq(url string) *http.Request {
	openReq, _ := http.NewRequest("GET", url, bytes.NewBuffer([]byte{}))
	q := openReq.URL.Query()
	q.Set("transport", "polling")
	openReq.URL.RawQuery = q.Encode()
	return openReq
}

func extractSid(body io.Reader) string {
	payload := parser.NewPayloadDecoder(body)
	packet, _ := payload.Next()
	openRes := map[string]interface{}{}
	json.NewDecoder(packet).Decode(&openRes)
	return openRes["sid"].(string)
}

// 读取数据从engine.io连接中
func read(conn Conn) (result []byte, err error) {
	_, r, err := conn.NextReader()
	if err != nil {
		return
	}
	result, err = ioutil.ReadAll(r)
	r.Close()
	return
}
func write(conn Conn, data []byte) (err error) {
	w, err := conn.NextWriter(MessageText)
	if err != nil {
		return
	}
	_, err = w.Write(data)
	w.Close()
	return
}
