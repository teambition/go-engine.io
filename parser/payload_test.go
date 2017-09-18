package parser

import (
	"bytes"
	"io"
	"runtime"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
)

func TestStringPayload(t *testing.T) {
	type packet struct {
		Type     PacketType
		Data     []byte
		IsString bool
	}
	type Test struct {
		name    string
		packets []packet
		output  string
	}
	var tests = []Test{
		{"all in one", []packet{packet{OPEN, nil, true}, packet{MESSAGE, []byte("测试"), true}, packet{MESSAGE, []byte("测试"), false}}, "\x31\x3a\x30\x37\x3a\x34\xe6\xb5\x8b\xe8\xaf\x95\x31\x30\x3a\x62\x34\x35\x72\x57\x4c\x36\x4b\x2b\x56"},
	}
	for _, test := range tests {
		buf := bytes.NewBuffer(nil)

		Convey("Given an array of packet "+test.name, t, func() {

			Convey("Create encoder", func() {
				encoder := NewPayloadEncoder(true)
				So(encoder.IsString(), ShouldBeTrue)

				Convey("Encoded", func() {
					for _, p := range test.packets {
						var e io.WriteCloser
						var err error
						if p.IsString {
							e, err = encoder.NextString(p.Type)
						} else {
							e, err = encoder.NextBinary(p.Type)
						}
						So(err, ShouldBeNil)
						for d := p.Data; len(d) > 0; {
							n, err := e.Write(d)
							So(err, ShouldBeNil)
							d = d[n:]
						}
						err = e.Close()
						So(err, ShouldBeNil)
					}

					Convey("End", func() {
						err := encoder.EncodeTo(buf)
						So(err, ShouldBeNil)
						So(buf.String(), ShouldEqual, test.output)
					})
				})
			})

			Convey("Create decoder", func() {
				decoder := NewPayloadDecoder(buf)

				Convey("Decode", func() {
					for i := 0; ; i++ {
						d, err := decoder.Next()
						if err == io.EOF {
							break
						}
						So(err, ShouldBeNil)
						So(d.Type(), ShouldEqual, test.packets[i].Type)

						if l := len(test.packets[i].Data); l > 0 {
							buf := make([]byte, len(test.packets[i].Data)+1)
							n, err := d.Read(buf)
							if n > 0 {
								So(err, ShouldBeNil)
								So(buf[:n], ShouldResemble, test.packets[i].Data)
							}
							_, err = d.Read(buf)
							So(err, ShouldEqual, io.EOF)
						}
						err = d.Close()
						So(err, ShouldBeNil)
					}
				})
			})
		})
	}
}

func TestBinaryPayload(t *testing.T) {
	type packet struct {
		Type     PacketType
		Data     []byte
		IsString bool
	}
	type Test struct {
		name    string
		packets []packet
		output  string
	}
	var tests = []Test{
		{"all in one", []packet{packet{OPEN, nil, true}, packet{MESSAGE, []byte("测试"), true}, packet{MESSAGE, []byte("测试"), false}}, "\x00\x01\xff\x30\x00\x07\xff\x34\xe6\xb5\x8b\xe8\xaf\x95\x01\x07\xff\x04\xe6\xb5\x8b\xe8\xaf\x95"},
	}
	for _, test := range tests {
		buf := bytes.NewBuffer(nil)

		Convey("Given an array of packet "+test.name, t, func() {

			Convey("Create encoder", func() {
				encoder := NewPayloadEncoder(false)
				So(encoder.IsString(), ShouldBeFalse)

				Convey("Encoded", func() {
					for _, p := range test.packets {
						var e io.WriteCloser
						var err error
						if p.IsString {
							e, err = encoder.NextString(p.Type)
						} else {
							e, err = encoder.NextBinary(p.Type)
						}
						So(err, ShouldBeNil)
						for d := p.Data; len(d) > 0; {
							n, err := e.Write(d)
							So(err, ShouldBeNil)
							d = d[n:]
						}
						err = e.Close()
						So(err, ShouldBeNil)
					}

					Convey("End", func() {
						err := encoder.EncodeTo(buf)
						So(err, ShouldBeNil)
						So(buf.String(), ShouldEqual, test.output)
					})
				})
			})

			Convey("Create decoder", func() {
				decoder := NewPayloadDecoder(buf)

				Convey("Decode", func() {
					for i := 0; ; i++ {
						d, err := decoder.Next()
						if err == io.EOF {
							break
						}
						So(err, ShouldBeNil)
						So(d.Type(), ShouldEqual, test.packets[i].Type)

						if l := len(test.packets[i].Data); l > 0 {
							buf := make([]byte, len(test.packets[i].Data)+1)
							n, err := d.Read(buf)
							if n > 0 {
								So(err, ShouldBeNil)
								So(buf[:n], ShouldResemble, test.packets[i].Data)
							}
							_, err = d.Read(buf)
							So(err, ShouldEqual, io.EOF)
						}
						err = d.Close()
						So(err, ShouldBeNil)
					}
				})
			})
		})
	}
}

func TestParallelEncode(t *testing.T) {
	assert := assert.New(t)

	prev := runtime.GOMAXPROCS(10)
	defer runtime.GOMAXPROCS(prev)

	c := make(chan int)
	max := 1000
	buf1 := bytes.NewBuffer(nil)
	buf2 := bytes.NewBuffer(nil)
	encoder := NewPayloadEncoder(true)
	for i := 0; i < max; i++ {
		go func() {
			e, _ := encoder.NextString(MESSAGE)
			e.Write([]byte("1234"))
			e.Close()
			c <- 1
		}()
	}
	for i := 0; i < max/2; i++ {
		<-c
	}
	err := encoder.EncodeTo(buf1)
	assert.Nil(err)
	for i := 0; i < max/2; i++ {
		<-c
	}
	err = encoder.EncodeTo(buf2)
	assert.Nil(err)

	for s := buf1.String(); len(s) > 0; {
		assert.True(strings.HasPrefix(s, "5:41234"))
		s = s[len("5:41234"):]
	}
	for s := buf2.String(); len(s) > 0; {
		assert.True(strings.HasPrefix(s, "5:41234"))
		s = s[len("5:41234"):]
	}
}
