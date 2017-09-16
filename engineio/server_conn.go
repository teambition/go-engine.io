package engineio

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/teambition/go-engine.io/message"
	"github.com/teambition/go-engine.io/parser"
	"github.com/teambition/go-engine.io/transport"
)

type MessageType message.MessageType
type state int

const (
	MessageBinary MessageType = MessageType(message.MessageBinary)
	MessageText   MessageType = MessageType(message.MessageText)

	stateNormal state = iota
	stateUpgrading
	stateClosing
	stateClosed
)

type transportCreaters map[string]transport.Creater

func (c transportCreaters) Get(name string) transport.Creater {
	return c[name]
}

type serverCallback interface {
	configure() config
	transports() transportCreaters
	onClose(sid string)
}

// Conn is the connection object of engine.io.
type Conn interface {

	// Id returns the session id of connection.
	Id() string

	// Request returns the first http request when established connection.
	Request() *http.Request

	// Close closes the connection.
	Close() error

	// NextReader returns the next message type, reader. If no message received, it will block.
	NextReader() (MessageType, io.ReadCloser, error)

	// NextWriter returns the next message writer with given message type.
	NextWriter(messageType MessageType) (io.WriteCloser, error)
	LastPing() time.Time
}

// InvalidError ...
var InvalidError = errors.New("invalid transport")

func newServerConn(id string, w http.ResponseWriter, r *http.Request, callback serverCallback) (*serverConn, error) {
	transportName := r.URL.Query().Get("transport")
	creater := callback.transports().Get(transportName)
	if creater.Name == "" {
		return nil, InvalidError
	}
	ret := &serverConn{
		id:           id,
		request:      r,
		callback:     callback,
		state:        stateNormal,
		readerChan:   make(chan *connReader),
		pingTimeout:  callback.configure().PingTimeout,
		pingInterval: callback.configure().PingInterval,
		lastPing:     time.Now(),
	}
	transport, err := creater.Server(w, r, ret)
	if err != nil {
		return nil, err
	}
	ret.setCurrent(transportName, transport)
	if err := ret.onOpen(); err != nil {
		return nil, err
	}

	return ret, nil
}

type serverConn struct {
	id              string
	request         *http.Request
	callback        serverCallback
	writerLocker    sync.Mutex
	transportLocker sync.RWMutex
	currentName     string
	current         transport.Server
	upgradingName   string
	upgrading       transport.Server
	state           state
	stateLocker     sync.RWMutex
	readerChan      chan *connReader
	closeReaderChan bool
	pingTimeout     time.Duration
	pingInterval    time.Duration
	pingLocker      sync.Mutex
	lastPing        time.Time
	packetLock      sync.Mutex
}

func (c *serverConn) Id() string {
	return c.id
}

func (c *serverConn) Request() *http.Request {
	return c.request
}

func (c *serverConn) NextReader() (MessageType, io.ReadCloser, error) {
	if c.getState() == stateClosed {
		return MessageBinary, nil, io.EOF
	}
	ret := <-c.readerChan
	if ret == nil {
		return MessageBinary, nil, io.EOF
	}
	return MessageType(ret.MessageType()), ret, nil
}

func (c *serverConn) NextWriter(t MessageType) (io.WriteCloser, error) {
	switch c.getState() {
	case stateUpgrading:
		for i := 0; i < 60; i++ {
			time.Sleep(50 * time.Millisecond)
			if c.getState() != stateUpgrading {
				break
			}
		}
		if c.getState() == stateUpgrading {
			return nil, fmt.Errorf("upgrading")
		}
	case stateNormal:
	default:
		return nil, io.EOF
	}
	c.writerLocker.Lock()
	current := c.getCurrent()
	if current == nil {
		c.writerLocker.Unlock()
		return nil, errors.New("current connection is nil")
	}
	ret, err := current.NextWriter(message.MessageType(t), parser.MESSAGE)
	if err != nil {
		c.writerLocker.Unlock()
		return ret, err
	}
	writer := newConnWriter(ret, &c.writerLocker)
	return writer, err
}

func (c *serverConn) Close() error {
	c.stateLocker.Lock()
	if c.state != stateNormal && c.state != stateUpgrading {
		c.stateLocker.Unlock()
		return nil
	}
	c.state = stateClosing
	c.stateLocker.Unlock()

	if c.upgrading != nil {
		c.upgrading.Close()
	}
	t := c.getCurrent()
	if t == nil {
		return nil
	}
	c.writerLocker.Lock()
	if w, err := t.NextWriter(message.MessageText, parser.CLOSE); err == nil {
		writer := newConnWriter(w, &c.writerLocker)
		writer.Close()
	} else {
		c.writerLocker.Unlock()
	}
	return t.Close()
}

func (c *serverConn) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	transportName := r.URL.Query().Get("transport")
	c.transportLocker.RLock()
	current := c.current
	currentName := c.currentName
	c.transportLocker.RUnlock()
	// Don't allow from websocket upgrade to polling
	if currentName == websocketProtocol && currentName != transportName {
		if r.Method == "POST" {
			parser.PollingPost(w, r, func(r *parser.PacketDecoder) {
				if r.Type() == parser.MESSAGE {
					c.OnPacket(r)
				}
			})
		} else {
			if err := parser.WriteNoop(w, r); err != nil {
				log.Println(err)
			}
		}
		return
	}
	if currentName != transportName {
		creater := c.callback.transports().Get(transportName)
		if creater.Name == "" {
			http.Error(w, fmt.Sprintf("invalid transport %s", transportName), http.StatusBadRequest)
			return
		}
		u, err := creater.Server(w, r, c)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		c.setUpgrading(creater.Name, u)
		return
	}
	current.ServeHTTP(w, r)
}

// Stack formats a stack trace of the calling goroutine
func Stack() string {
	buf := make([]byte, 2048)
	n := runtime.Stack(buf, false)
	return string(buf[:n])
}

func (c *serverConn) OnPacket(r *parser.PacketDecoder) {
	if s := c.getState(); s != stateNormal && s != stateUpgrading {
		return
	}
	c.packetLock.Lock()
	defer func() {
		c.packetLock.Unlock()
		if err := recover(); err != nil {
			log.Println(Stack())
		}
	}()
	switch r.Type() {
	case parser.OPEN:
	case parser.CLOSE:
		c.getCurrent().Close()
	case parser.PING:
		t := c.getCurrent()
		u := c.getUpgrade()
		if u != nil {
			t = u
		}
		c.writerLocker.Lock()
		if w, _ := t.NextWriter(message.MessageText, parser.PONG); w != nil {
			io.Copy(w, r)
			w.Close()
		}
		c.writerLocker.Unlock()
		if u != nil {
			go c.noopLoop()
		}
		c.setPing()
	case parser.MESSAGE:
		closeChan := make(chan struct{})
		c.readerChan <- newConnReader(r, closeChan)
		<-closeChan
		close(closeChan)
		r.Close()
	case parser.UPGRADE:
		c.upgraded()
	case parser.NOOP:
	}
}

func (c *serverConn) noopLoop() {
	upgradeTimes := 0
	for upgradeTimes < 300 {
		var t transport.Server
		c.transportLocker.RLock()
		if c.currentName == pollingProtocol && len(c.upgradingName) > 0 {
			t = c.current
		}
		c.transportLocker.RUnlock()
		if t == nil {
			return
		}
		c.writerLocker.Lock()
		w, err := t.NextWriter(message.MessageText, parser.NOOP)
		if w != nil {
			w.Close()
		}
		c.writerLocker.Unlock()
		if err != nil {
			return
		}
		upgradeTimes++
		time.Sleep(100 * time.Millisecond)
	}
}
func (c *serverConn) OnClose(server transport.Server) {
	if c.getState() == stateClosed {
		return
	}
	if c.getState() != stateClosing {
		if t := c.getUpgrade(); server == t {
			c.setUpgrading("", nil)
			return
		}
		t := c.getCurrent()
		if server != t {
			return
		}
		if t := c.getUpgrade(); t != nil {
			t.Close()
			c.setUpgrading("", nil)
		}
	}
	c.closeConn()
	c.callback.onClose(c.id)
}
func (c *serverConn) closeConn() {
	c.stateLocker.Lock()
	defer c.stateLocker.Unlock()
	if c.state != stateClosed {
		c.state = stateClosed
	}
	if c.closeReaderChan == false {
		c.closeReaderChan = true
		close(c.readerChan)
	}
}

func (c *serverConn) onOpen() error {
	upgrades := []string{}
	for name := range c.callback.transports() {
		if name == c.currentName || c.currentName == websocketProtocol {
			continue
		}
		upgrades = append(upgrades, name)
	}
	type connectionInfo struct {
		Sid          string        `json:"sid"`
		Upgrades     []string      `json:"upgrades"`
		PingInterval time.Duration `json:"pingInterval"`
		PingTimeout  time.Duration `json:"pingTimeout"`
	}
	resp := connectionInfo{
		Sid:          c.Id(),
		Upgrades:     upgrades,
		PingInterval: c.callback.configure().PingInterval / time.Millisecond,
		PingTimeout:  c.callback.configure().PingTimeout / time.Millisecond,
	}
	c.writerLocker.Lock()
	defer c.writerLocker.Unlock()
	w, err := c.getCurrent().NextWriter(message.MessageText, parser.OPEN)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(w)
	if err := encoder.Encode(resp); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return nil
}

func (c *serverConn) getCurrent() transport.Server {
	c.transportLocker.RLock()
	defer c.transportLocker.RUnlock()

	return c.current
}
func (c *serverConn) getUpgrade() transport.Server {
	c.transportLocker.RLock()
	defer c.transportLocker.RUnlock()

	return c.upgrading
}

func (c *serverConn) setCurrent(name string, s transport.Server) {
	c.transportLocker.Lock()
	defer c.transportLocker.Unlock()

	c.currentName = name
	c.current = s
}

func (c *serverConn) setUpgrading(name string, s transport.Server) {
	c.transportLocker.Lock()
	defer c.transportLocker.Unlock()

	c.upgradingName = name
	c.upgrading = s
	c.setState(stateUpgrading)
}

func (c *serverConn) upgraded() {
	c.transportLocker.Lock()
	if c.upgrading == nil {
		c.transportLocker.Unlock()
		return
	}
	current := c.current
	c.current = c.upgrading
	c.currentName = c.upgradingName
	c.upgrading = nil
	c.upgradingName = ""

	c.setState(stateNormal)
	c.transportLocker.Unlock()

	current.Close()
}

func (c *serverConn) getState() state {
	c.stateLocker.RLock()
	defer c.stateLocker.RUnlock()
	return c.state
}

func (c *serverConn) setState(state state) {
	c.stateLocker.Lock()
	defer c.stateLocker.Unlock()
	c.state = state
}

func (c *serverConn) LastPing() time.Time {
	c.pingLocker.Lock()
	defer c.pingLocker.Unlock()
	return c.lastPing
}
func (c *serverConn) setPing() {
	c.pingLocker.Lock()
	defer c.pingLocker.Unlock()
	c.lastPing = time.Now()
}
