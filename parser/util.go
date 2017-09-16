package parser

import (
	"bytes"
	"io"
	"net/http"
)

// PollingPost ...
func PollingPost(w http.ResponseWriter, r *http.Request, onPacket func(r *PacketDecoder)) {
	w.Header().Set("Content-Type", "text/html")
	var decoder *PayloadDecoder
	if j := r.URL.Query().Get("j"); j != "" {
		// JSONP Polling
		d := r.FormValue("d")
		decoder = NewPayloadDecoder(bytes.NewBufferString(d))
	} else {
		// XHR Polling
		decoder = NewPayloadDecoder(r.Body)
	}
	for {
		d, err := decoder.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		onPacket(d)
		d.Close()
	}
	w.Write([]byte("ok"))
}

// WriteNoop ...
func WriteNoop(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
	encoder := NewPayloadEncoder(r.URL.Query()["b64"] != nil)
	ret, err := encoder.NextString(NOOP)
	if err != nil {
		return err
	}
	err = ret.Close()
	if err != nil {
		return err
	}
	return encoder.EncodeTo(w)
}
