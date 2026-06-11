// Gerador de tráfego isolado pra observar a estação 06-mapping.
//
// Um único processo que segura, vivo e ao mesmo tempo:
//   - 1 socket LISTEN  (servidor)
//   - N sockets ESTABLISHED do lado cliente  (disca em si mesmo)
//   - N sockets ESTABLISHED do lado servidor  (aceita as próprias conexões)
//
// Total: 1 + 2N sockets, TODOS donos do mesmo PID. É o cenário ideal pro
// mapper: a tabela enche e cada linha resolve pra um PID conhecido, em vez do
// `-` que aparece quando o socket não tem processo dono vivo.
//
// Rode no container server, deixe vivo, e rode o 06-mapping em outro shell.

package main

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"time"
)

const (
	addr  = "127.0.0.1:9090"
	conns = 3 // quantos pares cliente↔servidor abrir
)

func main() {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "listen: %v\n", err)
		os.Exit(1)
	}
	defer ln.Close()

	// Guarda os sockets aceitos (lado servidor). Manter a referência viva
	// impede o GC de fechar a conexão — o kernel a mantém em ESTABLISHED.
	var serverSide []net.Conn
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return // listener fechou
			}
			serverSide = append(serverSide, c)
		}
	}()

	// Disca em si mesmo N vezes (lado cliente). Cada Dial completa um 3-way
	// handshake → vira ESTABLISHED dos dois lados.
	var clientSide []net.Conn
	for i := 0; i < conns; i++ {
		c, err := net.Dial("tcp", addr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "dial: %v\n", err)
			os.Exit(1)
		}
		clientSide = append(clientSide, c)
	}

	// Dá um instante pro accept loop registrar os lados servidor.
	time.Sleep(100 * time.Millisecond)

	fmt.Printf("PID %d segurando %d sockets em %s:\n", os.Getpid(), 1+2*conns, addr)
	fmt.Printf("  1 LISTEN + %d ESTABLISHED (cliente) + %d ESTABLISHED (servidor)\n", conns, len(serverSide))
	fmt.Println("Rode o 06-mapping em outro shell do server. Ctrl-C pra sair.")

	// Bloqueia em sinal (não em select{}, que dispararia o detector de deadlock
	// se todas as goroutines parassem). Ctrl-C fecha tudo via defer.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig
}
