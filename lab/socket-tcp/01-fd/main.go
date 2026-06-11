// Estação 01-fd: socket = file descriptor no kernel.
//
// Objetivo: ver, com seus olhos, que o socket criado por net.Listen é um FD
// que o kernel registrou em /proc/<pid>/fd/.
//
// O que o programa faz:
//   1. Chama net.Listen("tcp", ":8080") — internamente o kernel faz socket()+bind()+listen().
//   2. Extrai o FD do socket via syscall.Conn.
//   3. Imprime PID e FD pra você inspecionar em outro shell.
//   4. Fica vivo até Ctrl+C.

package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	// net.TCPListener implementa SyscallConn — é como acessamos o FD bruto.
	tcpL := listener.(*net.TCPListener)
	rawConn, err := tcpL.SyscallConn()
	if err != nil {
		log.Fatalf("syscall conn: %v", err)
	}

	var fd uintptr
	err = rawConn.Control(func(f uintptr) {
		fd = f
	})
	if err != nil {
		log.Fatalf("control: %v", err)
	}

	pid := os.Getpid()
	addr := listener.Addr().String()

	fmt.Println("=== Estação 01: socket = FD ===")
	fmt.Printf("PID:           %d\n", pid)
	fmt.Printf("Listener FD:   %d\n", fd)
	fmt.Printf("Listener Addr: %s\n", addr)
	fmt.Println()
	fmt.Println("Em outro shell do MESMO container (server):")
	fmt.Printf("  ls -la /proc/%d/fd/\n", pid)
	fmt.Printf("  readlink /proc/%d/fd/%d\n", pid, fd)
	fmt.Println()
	fmt.Println("No inspector (compartilha NET namespace):")
	fmt.Println("  ss -tln")
	fmt.Println("  cat /proc/net/tcp")
	fmt.Println()
	fmt.Println("Ctrl+C para sair.")

	// Não chama Accept. Listener fica em LISTEN parado.
	// Bloqueia em SIGINT/SIGTERM pra evitar deadlock detector do Go
	// (única goroutine viva sem possibilidade de progresso seria considerada deadlock).
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	<-sigs
}
