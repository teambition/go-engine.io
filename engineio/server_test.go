package engineio

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"testing"
	"time"

	wsclient "github.com/mushroomsir/engine.io"
	"github.com/mushroomsir/engine.io/transports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/teambition/go-engine.io/client"
	"github.com/teambition/go-engine.io/parser"
)

func TestWebsocket(t *testing.T) {
	assert := assert.New(t)

	client, _ := wsclient.NewClient(testWSURL + "?transport=websocket")
	event := <-client.Event
	assert.Equal("open", event.Type)
	assert.Contains(string(event.Data), `"upgrades":[],"pingInterval":25000,"pingTimeout":60000}`)
	assert.NotNil(event.Data)
	for i := 0; i < 100; i++ {
		client.SendMessage(testBytes)
		event := <-client.Event
		assert.Equal("message", event.Type)
		assert.Equal(testBytes, event.Data)
	}
}

func TestWebsocketPing(t *testing.T) {
	assert := assert.New(t)
	client, _ := wsclient.NewClient(testWSURL + "?transport=websocket")

	event := <-client.Event
	assert.Equal("open", event.Type)
	assert.Contains(string(event.Data), `"upgrades":[],"pingInterval":25000,"pingTimeout":60000}`)

	for i := 0; i < 100; i++ {
		client.SendPacket(&transports.Packet{Type: transports.Ping})
		event = <-client.Event
		assert.Equal("pong", event.Type)
	}
}

func TestPollingPing(t *testing.T) {
	assert := assert.New(t)
	info := pollingHandShake(t)

	req, err := http.NewRequest("POST", testURL+"?transport=polling&sid="+info.Sid, nil)
	assert.Nil(err)
	polling, err := client.NewPolling(req)
	assert.Nil(err)
	err = writePing(polling)
	assert.Nil(err)
}
func TestUpgrades(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	//====== Start =====
	req, err := http.NewRequest("GET", testURL+"?transport=polling", nil)
	require.Nil(err)
	polling, err := client.NewPolling(req)
	assert.Nil(err)
	packet, err := polling.NextReader()
	require.Nil(err)
	result := readPacket(packet)
	var info connectionInfo
	err = json.Unmarshal([]byte(result), &info)
	require.Nil(err)
	require.Contains(result, `"upgrades":["websocket"],"pingInterval":25000,"pingTimeout":60000}`)
	require.Equal(parser.OPEN, packet.Type())
	require.NotEmpty(info.Sid)
	packet.Close()
	//====== End =====
	go func() {
		wsclient, err := wsclient.NewClient(testWSURL + "?transport=websocket&sid=" + info.Sid)
		require.Nil(err)
		wsclient.SendPacket(&transports.Packet{Type: transports.Ping, Data: []byte("probe")})
		event := <-wsclient.Event
		assert.Equal("pong", event.Type)
		assert.Equal("probe", string(event.Data))

		wsclient.SendPacket(&transports.Packet{Type: transports.Upgrade})

		for i := 0; i < 1; i++ {
			req, err := http.NewRequest("POST", testURL+"?transport=polling&sid="+info.Sid, nil)
			assert.Nil(err)
			polling, err := client.NewPolling(req)
			assert.Nil(err)
			err = writeMsgPacket(polling, testData)
			assert.Nil(err)

			event := <-wsclient.Event
			assert.Equal(testData, string(event.Data))

			wsclient.SendMessage(testBytes)
			event = <-wsclient.Event
			assert.Equal(testBytes, event.Data)

			// ws ping
			wsclient.SendPacket(&transports.Packet{Type: transports.Ping})
			event = <-wsclient.Event
			assert.Equal("pong", event.Type)

			// polling ping, server will ignore it
			req, err = http.NewRequest("POST", testURL+"?transport=polling&sid="+info.Sid, nil)
			assert.Nil(err)
			polling, err = client.NewPolling(req)
			assert.Nil(err)
			err = writePing(polling)
			assert.Nil(err)
		}
	}()
	go func() {
		for i := 0; i < 1; i++ {
			req, err = http.NewRequest("GET", testURL+"?transport=polling&sid="+info.Sid, nil)
			require.Nil(err)
			polling, err = client.NewPolling(req)
			assert.Nil(err)
			packet, err = polling.NextReader()
			require.Nil(err)
			result = readPacket(packet)
			assert.Equal(200, polling.Response().StatusCode)
			assert.Equal(result, "")
		}
	}()

}

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
