package main

import (
	"crypto/tls"
	"flag"
	"io"
	"log"
	"net"
)

func main() {
	var (
		listenAddr string
		targetAddr string
		certFile   string
		keyFile    string
	)
	flag.StringVar(&listenAddr, "listen", ":5443", "listen address")
	flag.StringVar(&targetAddr, "target", "127.0.0.1:5000", "target address")
	flag.StringVar(&certFile, "cert", "../certs/cert.pem", "tls cert")
	flag.StringVar(&keyFile, "key", "../certs/key.pem", "tls key")
	flag.Parse()

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		log.Fatalf("load cert: %v", err)
	}
	cfg := &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}

	ln, err := tls.Listen("tcp", listenAddr, cfg)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	log.Printf("tls proxy listen=%s -> target=%s", listenAddr, targetAddr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("accept: %v", err)
			continue
		}
		go handle(conn, targetAddr)
	}
}

func handle(c net.Conn, target string) {
	defer c.Close()
	up, err := net.Dial("tcp", target)
	if err != nil {
		return
	}
	defer up.Close()

	go io.Copy(up, c)
	io.Copy(c, up)
}
