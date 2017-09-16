package client

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"github.com/teambition/go-engine.io/message"
	"github.com/teambition/go-engine.io/parser"
)

type state int

const (
	stateNormal state = iota
	stateClosing
	stateClosed
)

type Polling struct {
	req            http.Request
	url            url.URL
	seq            uint
	getResp        *http.Response
	postResp       *http.Response
	resp           *http.Response
	payloadDecoder *parser.PayloadDecoder
	payloadEncoder *parser.PayloadEncoder
	client         *http.Client
	state          state
}

func NewPolling(r *http.Request) (Client, error) {
	ret := &Polling{
		req:            *r,
		url:            *r.URL,
		seq:            0,
		payloadEncoder: parser.NewPayloadEncoder(r.URL.Query()["b64"] != nil),
		client:         http.DefaultClient,
		state:          stateNormal,
	}
	return ret, nil
}

func (c *Polling) Response() *http.Response {
	return c.resp
}

func (c *Polling) NextReader() (*parser.PacketDecoder, error) {
	if c.state != stateNormal {
		return nil, io.EOF
	}
	if c.payloadDecoder != nil {
		ret, err := c.payloadDecoder.Next()
		if err != io.EOF {
			return ret, err
		}
		c.getResp.Body.Close()
		c.payloadDecoder = nil
	}
	req := c.getReq()
	req.Method = "GET"
	var err error
	c.getResp, err = c.client.Do(req)
	if err != nil {
		return nil, err
	}
	if c.resp == nil {
		c.resp = c.getResp
	}
	if c.resp.StatusCode != 200 {
		result, _ := ioutil.ReadAll(c.getResp.Body)
		panic(string(result))
	}
	c.payloadDecoder = parser.NewPayloadDecoder(c.getResp.Body)

	return c.payloadDecoder.Next()
}

func (c *Polling) NextWriter(messageType message.MessageType, packetType parser.PacketType) (io.WriteCloser, error) {
	if c.state != stateNormal {
		return nil, io.EOF
	}
	next := c.payloadEncoder.NextBinary
	if messageType == message.MessageText {
		next = c.payloadEncoder.NextString
	}
	w, err := next(packetType)
	if err != nil {
		return nil, err
	}
	return newClientWriter(c, w), nil
}

func (c *Polling) Close() error {
	if c.state != stateNormal {
		return nil
	}
	c.state = stateClosed
	return nil
}

func (c *Polling) getReq() *http.Request {
	req := c.req
	url := c.url
	req.URL = &url
	query := req.URL.Query()
	query.Set("t", fmt.Sprintf("%d-%d", time.Now().Unix()*1000, c.seq))
	c.seq++
	req.URL.RawQuery = query.Encode()
	return &req
}

func (c *Polling) doPost() error {
	if c.state != stateNormal {
		return io.EOF
	}
	req := c.getReq()
	req.Method = "POST"
	buf := bytes.NewBuffer(nil)
	if err := c.payloadEncoder.EncodeTo(buf); err != nil {
		return err
	}
	req.Body = ioutil.NopCloser(buf)
	var err error
	c.postResp, err = c.client.Do(req)
	if err != nil {
		return err
	}
	if c.resp == nil {
		c.resp = c.postResp
	}
	return nil
}

type clientWriter struct {
	io.WriteCloser
	client *Polling
}

func newClientWriter(c *Polling, w io.WriteCloser) io.WriteCloser {
	return &clientWriter{
		WriteCloser: w,
		client:      c,
	}
}

func (w *clientWriter) Close() error {
	if err := w.WriteCloser.Close(); err != nil {
		return err
	}
	return w.client.doPost()
}
