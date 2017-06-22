package transport

import (
	"io"
	"net/http"

	"github.com/teambition/go-engine.io/message"
	"github.com/teambition/go-engine.io/parser"
)

type Callback interface {
	OnPacket(r *parser.PacketDecoder)
	OnClose(server Server)
}

type Creater struct {
	Name      string
	Upgrading bool
	Server    func(w http.ResponseWriter, r *http.Request, callback Callback) (Server, error)
}

// Server is a transport layer in server to connect client.
type Server interface {

	// ServeHTTP handles the http request. It will call conn.onPacket when receive packet.
	ServeHTTP(http.ResponseWriter, *http.Request)

	// Close closes the transport.
	Close() error

	// NextWriter returns packet writer. This function call should be synced.
	NextWriter(messageType message.MessageType, packetType parser.PacketType) (io.WriteCloser, error)
}
