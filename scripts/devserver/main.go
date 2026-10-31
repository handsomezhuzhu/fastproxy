// Command devserver starts a local fake upstream HTTP proxy plus a small
// HTTP target for container e2e testing:
//
//	upstream proxy: 127.0.0.1:18180 (forwards CONNECT + absolute-form GETs)
//	target site:    127.0.0.1:18181 (replies "hello-from-devserver-target")
package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
)

const (
	// upstreamAddr binds all interfaces so Docker containers can reach the
	// host devserver via host.docker.internal or the bridge gateway IP.
	upstreamAddr = ":18180"
	targetAddr   = "127.0.0.1:18181"
)

func main() {
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "hello-from-devserver-target")
		})
		if err := http.ListenAndServe(targetAddr, mux); err != nil {
			fmt.Println("target:", err)
		}
	}()

	ln, err := net.Listen("tcp", upstreamAddr)
	if err != nil {
		panic(err)
	}
	fmt.Printf("devserver ready: upstream=%s target=%s\n", upstreamAddr, targetAddr)
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go handle(c)
	}
}

// pipeConn reads through a buffered reader (possible leftover bytes) and
// writes to the raw connection.
type pipeConn struct {
	br *bufio.Reader
	net.Conn
}

func (c *pipeConn) Read(p []byte) (int, error) { return c.br.Read(p) }

func handle(conn net.Conn) {
	defer conn.Close()
	br := bufio.NewReader(conn)
	req, err := http.ReadRequest(br)
	if err != nil {
		return
	}
	if req.Method == http.MethodConnect {
		fmt.Fprint(conn, "HTTP/1.1 200 Connection Established\r\n\r\n")
		up, err := net.Dial("tcp", req.Host)
		if err != nil {
			fmt.Println("upstream dial target:", err)
			return
		}
		defer up.Close()
		cc := &pipeConn{br: br, Conn: conn}
		fin := make(chan struct{}, 2)
		go func() { io.Copy(up, cc); fin <- struct{}{} }()
		go func() { io.Copy(cc, up); fin <- struct{}{} }()
		<-fin
		up.Close()
		conn.Close()
		return
	}
	resp, err := (&http.Transport{}).RoundTrip(req)
	if err != nil {
		fmt.Println("upstream roundtrip:", err)
		return
	}
	defer resp.Body.Close()
	resp.Close = true
	resp.Write(conn)
}
