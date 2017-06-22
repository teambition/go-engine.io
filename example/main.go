package main

import (
	"encoding/hex"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	_ "net/http/pprof"

	"github.com/teambition/go-engine.io/engineio"
)

func main() {
	server, err := engineio.NewServer(nil)
	if err != nil {
		log.Fatal(err)
	}
	server.SetPingInterval(time.Second * 2)
	server.SetPingTimeout(time.Second * 3)

	go func() {
		for {
			conn, _ := server.Accept()
			go func() {
				log.Println("connected:", conn.Id())
				defer func() {
					conn.Close()
					log.Println("disconnected:", conn.Id())
				}()
				for {
					messageType, r, err := conn.NextReader()
					if err != nil {
						return
					}
					b, err := ioutil.ReadAll(r)
					if err != nil {
						return
					}
					r.Close()
					if messageType == engineio.MessageText {
						log.Println(messageType, string(b))
					} else {
						log.Println(messageType, hex.EncodeToString(b))
					}
					w, err := conn.NextWriter(messageType)
					if err != nil {
						return
					}
					w.Write(b)
					w.Close()
				}
			}()
		}
	}()

	http.Handle("/engine.io/", server)
	http.Handle("/", http.FileServer(http.Dir("./web")))
	log.Println("Serving at localhost:4000...")
	log.Fatal(http.ListenAndServe(":4000", nil))
}
