package client

import (
	"io"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/teambition/go-engine.io/message"
	"github.com/teambition/go-engine.io/parser"
)

// Client is a transport layer in client to connect server.
type Client interface {

	// Response returns the response of last http request.
	Response() *http.Response

	// NextReader returns packet decoder. This function call should be synced.
	NextReader() (*parser.PacketDecoder, error)

	// NextWriter returns packet writer. This function call should be synced.
	NextWriter(messageType message.MessageType, packetType parser.PacketType) (io.WriteCloser, error)

	// Close closes the transport.
	Close() error
}

// NewWebsocket ...
func NewWebsocket(r *http.Request) (Client, error) {
	dialer := websocket.DefaultDialer

	conn, resp, err := dialer.Dial(r.URL.String(), r.Header)
	if err != nil {
		return nil, err
	}

	return &Websocket{
		Conn: conn,
		resp: resp,
	}, nil
}

// Websocket ...
type Websocket struct {
	Conn *websocket.Conn
	resp *http.Response
}

// Response ...
func (c *Websocket) Response() *http.Response {
	return c.resp
}

// NextReader ...
func (c *Websocket) NextReader() (*parser.PacketDecoder, error) {
	var reader io.Reader
	for {
		t, r, err := c.Conn.NextReader()
		if err != nil {
			return nil, err
		}
		switch t {
		case websocket.TextMessage:
			fallthrough
		case websocket.BinaryMessage:
			reader = r
			return parser.NewDecoder(reader)
		}
	}
}

// NextWriter ...
func (c *Websocket) NextWriter(msgType message.MessageType, packetType parser.PacketType) (io.WriteCloser, error) {
	wsType, newEncoder := websocket.TextMessage, parser.NewStringEncoder
	if msgType == message.MessageBinary {
		wsType, newEncoder = websocket.BinaryMessage, parser.NewBinaryEncoder
	}

	w, err := c.Conn.NextWriter(wsType)
	if err != nil {
		return nil, err
	}
	ret, err := newEncoder(w, packetType)
	if err != nil {
		return nil, err
	}
	return ret, nil
}

// Close ...
func (c *Websocket) Close() error {
	return c.Conn.Close()
}
