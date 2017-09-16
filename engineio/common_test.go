package engineio

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/teambition/go-engine.io/client"
	"github.com/teambition/go-engine.io/message"
	"github.com/teambition/go-engine.io/parser"
)

var (
	testServer *httptest.Server
	testURL    string
	testWSURL  string
)

func TestMain(m *testing.M) {
	server, err := NewServer(nil)
	if err != nil {
		log.Fatal(err)
	}
	go func() {
		for {
			conn, _ := server.Accept()
			go distributer(conn)
		}
	}()

	http.Handle("/engine.io/", server)
	testServer = httptest.NewServer(nil)
	testURL = testServer.URL + "/engine.io/"
	testWSURL = strings.Replace(testServer.URL, "http", "ws", 1) + "/engine.io/"
	retCode := m.Run()
	testServer.Close()
	os.Exit(retCode)
}
func distributer(conn Conn) {
	log.Println("connected:", conn.Id())
	defer func() {
		conn.Close()
		log.Println("disconnected:", conn.Id())
	}()
	for {
		messageType, r, err := conn.NextReader()
		if err != nil {
			log.Println(err)
			return
		}
		b, err := ioutil.ReadAll(r)
		if err != nil {
			log.Println(err)
			return
		}
		err = r.Close()
		if err != nil {
			log.Println(err)
		}
		if messageType == MessageText {
			//	log.Println(messageType, string(b))
		} else {
			//	log.Println(messageType, hex.EncodeToString(b))
		}
		w, err := conn.NextWriter(messageType)
		if err != nil {
			log.Println(err)
			return
		}
		_, err = w.Write(b)
		if err != nil {
			log.Println(err)
		}
		err = w.Close()
		if err != nil {
			log.Println(err)
		}
	}
}

func writeMsgPacket(cl client.Client, data string) error {
	return writePacket(cl, parser.MESSAGE, data)
}
func writePing(cl client.Client) error {
	return writePacket(cl, parser.PING, "")
}
func writePacket(cl client.Client, packetType parser.PacketType, data string) error {
	w, err := cl.NextWriter(message.MessageText, packetType)
	if err != nil {
		return err
	}
	_, err = w.Write([]byte(data))
	if err != nil {
		w.Close()
		return err
	}
	return w.Close()
}

func readPacket(packet *parser.PacketDecoder) string {
	result, err := ioutil.ReadAll(packet)
	if err != nil {
		return ""
	}
	return string(result)
}

type connectionInfo struct {
	Sid          string        `json:"sid"`
	Upgrades     []string      `json:"upgrades"`
	PingInterval time.Duration `json:"pingInterval"`
	PingTimeout  time.Duration `json:"pingTimeout"`
}

func pollingHandShake(t *testing.T) *connectionInfo {
	assert := assert.New(t)
	require := require.New(t)

	req, err := http.NewRequest("GET", testURL+"?transport=polling", nil)
	require.Nil(err)
	assert.NotNil(req)
	ws, err := client.NewPolling(req)
	assert.Nil(err)
	packet, err := ws.NextReader()
	require.Nil(err)
	result := readPacket(packet)
	var info connectionInfo
	err = json.Unmarshal([]byte(result), &info)
	require.Nil(err)
	require.Contains(result, `"upgrades":["websocket"],"pingInterval":25000,"pingTimeout":60000}`)
	require.Equal(parser.OPEN, packet.Type())
	require.NotEmpty(info.Sid)
	packet.Close()
	return &info
}
