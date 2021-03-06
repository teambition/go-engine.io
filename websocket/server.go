package websocket

import (
	"io"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/teambition/go-engine.io/message"
	"github.com/teambition/go-engine.io/parser"
	"github.com/teambition/go-engine.io/transport"
)

type Server struct {
	callback transport.Callback
	conn     *websocket.Conn
}

func NewServer(w http.ResponseWriter, r *http.Request, callback transport.Callback) (transport.Server, error) {
	conn, err := websocket.Upgrade(w, r, w.Header(), 10240, 10240)
	if err != nil {
		return nil, err
	}

	ret := &Server{
		callback: callback,
		conn:     conn,
	}

	go ret.serveHTTP(w, r)

	return ret, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		parser.PollingPost(w, r, func(r *parser.PacketDecoder) {
			if r.Type() == parser.MESSAGE {
				s.callback.OnPacket(r)
			}
		})
	}
}

func (s *Server) NextWriter(msgType message.MessageType, packetType parser.PacketType) (io.WriteCloser, error) {
	wsType, newEncoder := websocket.TextMessage, parser.NewStringEncoder
	if msgType == message.MessageBinary {
		wsType, newEncoder = websocket.BinaryMessage, parser.NewBinaryEncoder
	}

	w, err := s.conn.NextWriter(wsType)
	if err != nil {
		return nil, err
	}
	ret, err := newEncoder(w, packetType)
	if err != nil {
		return nil, err
	}
	return ret, nil
}

func (s *Server) Close() error {
	return s.conn.Close()
}

func (s *Server) serveHTTP(w http.ResponseWriter, r *http.Request) {
	defer s.callback.OnClose(s)

	for {
		t, r, err := s.conn.NextReader()
		if err != nil {
			s.conn.Close()
			return
		}

		switch t {
		case websocket.TextMessage:
			fallthrough
		case websocket.BinaryMessage:
			decoder, err := parser.NewDecoder(r)
			if err != nil {
				return
			}
			s.callback.OnPacket(decoder)
			decoder.Close()
		}
	}
}
