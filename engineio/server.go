package engineio

import (
	"bytes"
	"crypto/md5"
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	"github.com/teambition/go-engine.io/polling"
	"github.com/teambition/go-engine.io/transport"
	"github.com/teambition/go-engine.io/websocket"
)

type config struct {
	PingTimeout   time.Duration
	PingInterval  time.Duration
	AllowRequest  func(*http.Request) error
	AllowUpgrades bool
	Cookie        string
	NewId         func(r *http.Request) (string, error)
}

// Server is the server of engine.io.
type Server struct {
	config         config
	socketChan     chan Conn
	serverSessions Sessions
	creaters       transportCreaters
}

var websocketProtocol = "websocket"
var pollingProtocol = "polling"

// NewServer returns the server suppported given transports. If transports is nil, server will use ["polling", "websocket"] as default.
func NewServer(transports []string) (*Server, error) {
	if transports == nil {
		transports = []string{pollingProtocol, websocketProtocol}
	}
	creaters := make(transportCreaters)
	for _, t := range transports {
		switch t {
		case pollingProtocol:
			creaters[t] = transport.Creater{
				Name:   pollingProtocol,
				Server: polling.NewServer,
			}
		case websocketProtocol:
			creaters[t] = transport.Creater{
				Name:   websocketProtocol,
				Server: websocket.NewServer,
			}
		default:
			return nil, InvalidError
		}
	}
	return &Server{
		config: config{
			PingTimeout:   60000 * time.Millisecond,
			PingInterval:  25000 * time.Millisecond,
			AllowRequest:  func(*http.Request) error { return nil },
			AllowUpgrades: true,
			Cookie:        "io",
			NewId:         newId,
		},
		socketChan:     make(chan Conn),
		serverSessions: newServerSessions(),
		creaters:       creaters,
	}, nil
}

// SetPingTimeout sets the timeout of ping. When time out, server will close connection. Default is 60s.
func (s *Server) SetPingTimeout(t time.Duration) {
	s.config.PingTimeout = t
}

// SetPingInterval sets the interval of ping. Default is 25s.
func (s *Server) SetPingInterval(t time.Duration) {
	s.config.PingInterval = t
}

// Count returns a count of current number of active connections in session
func (s *Server) Count() int {
	return s.serverSessions.Len()
}

// SetAllowRequest sets the middleware function when establish connection. If it return non-nil, connection won't be established. Default will allow all request.
func (s *Server) SetAllowRequest(f func(*http.Request) error) {
	s.config.AllowRequest = f
}

// SetAllowUpgrades sets whether server allows transport upgrade. Default is true.
func (s *Server) SetAllowUpgrades(allow bool) {
	s.config.AllowUpgrades = allow
}

// SetCookie sets the name of cookie which used by engine.io. Default is "io".
func (s *Server) SetCookie(prefix string) {
	s.config.Cookie = prefix
}

// SetNewId sets the callback func to generate new connection id. By default, id is generated from remote addr + current time stamp
func (s *Server) SetNewId(f func(*http.Request) (string, error)) {
	s.config.NewId = f
}

// SetSessionManager sets the sessions as server's session manager. Default sessions is single process manager. You can custom it as load balance.
func (s *Server) SetSessionManager(sessions Sessions) {
	s.serverSessions = sessions
}

// ServeHTTP handles http request.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	sid := r.URL.Query().Get("sid")
	conn := s.serverSessions.Get(sid)
	if conn == nil {
		if sid != "" {
			http.Error(w, "invalid sid", http.StatusBadRequest)
			return
		}

		if err := s.config.AllowRequest(r); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var err error
		sid, err = s.config.NewId(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if sid == "" {
			http.Error(w, "empty sid", http.StatusBadRequest)
			return
		}
		conn, err = newServerConn(sid, w, r, s)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.serverSessions.Set(sid, conn)
		s.socketChan <- conn
	}
	http.SetCookie(w, &http.Cookie{
		Name:  s.config.Cookie,
		Value: sid,
	})

	conn.(*serverConn).ServeHTTP(w, r)
}

// Accept returns Conn when client connect to server.
func (s *Server) Accept() (Conn, error) {
	return <-s.socketChan, nil
}

func (s *Server) configure() config {
	return s.config
}

func (s *Server) transports() transportCreaters {
	return s.creaters
}

func (s *Server) onClose(id string) {
	s.serverSessions.Remove(id)
}

func newId(r *http.Request) (string, error) {
	hash := fmt.Sprintf("%s %s", r.RemoteAddr, time.Now())
	buf := bytes.NewBuffer(nil)
	sum := md5.Sum([]byte(hash))
	encoder := base64.NewEncoder(base64.URLEncoding, buf)
	encoder.Write(sum[:])
	encoder.Close()
	return buf.String()[:20], nil
}
