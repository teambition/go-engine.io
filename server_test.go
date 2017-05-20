package engineio

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/teambition/go-engine.io/parser"
)

func TestServer(t *testing.T) {
	Convey("Setup server", t, func() {
		server, err := NewServer(nil)
		So(err, ShouldBeNil)
		server.SetPingInterval(time.Second)
		So(server.config.PingInterval, ShouldEqual, time.Second)
		server.SetPingTimeout(10 * time.Second)
		So(server.config.PingTimeout, ShouldEqual, 10*time.Second)
		f := func(*http.Request) error { return nil }
		server.SetAllowRequest(f)
		So(server.config.AllowRequest, ShouldEqual, f)
		server.SetAllowUpgrades(false)
		So(server.config.AllowUpgrades, ShouldBeFalse)
		server.SetCookie("prefix")
		So(server.config.Cookie, ShouldEqual, "prefix")
	})

	Convey("Create server", t, func() {

		Convey("Test new id", func() {
			req, err := http.NewRequest("GET", "/", nil)
			So(err, ShouldBeNil)
			id1 := newId(req)
			id2 := newId(req)
			So(id1, ShouldNotEqual, id2)
		})

	})
}

func newOpenReq() *http.Request {
	openReq, _ := http.NewRequest("GET", "/", bytes.NewBuffer([]byte{}))
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
