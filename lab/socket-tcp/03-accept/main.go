// Estação 03-accept: listener ≠ conexão.
//
// Server loga FD do listener no startup e FD de cada conexão aceita.
// Você verá: listener tem 1 FD; cada Accept retorna FD novo.

package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"syscall"
)

// rawFD extrai o file descriptor bruto de qualquer coisa que tenha SyscallConn().
func rawFD(rawSC interface {
	SyscallConn() (syscall.RawConn, error)
}) uintptr {
	rc, err := rawSC.SyscallConn()
	if err != nil {
		log.Fatalf("syscall conn: %v", err)
	}
	var fd uintptr
	err = rc.Control(func(f uintptr) { fd = f })
	if err != nil {
		log.Fatalf("control: %v", err)
	}
	return fd
}

func main() {
	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	listenerFD := rawFD(listener.(*net.TCPListener))

	fmt.Println("=== Estação 03: listener vs conexão ===")
	fmt.Printf("PID:           %d\n", os.Getpid())
	fmt.Printf("Listener FD:   %d  <- socket que escuta\n", listenerFD)
	fmt.Println()
	fmt.Println("Conecte clientes:")
	fmt.Println("  nc localhost 8080")
	fmt.Println()

	connID := 0
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("accept: %v", err)
			continue
		}
		connID++

		connFD := rawFD(conn.(*net.TCPConn))
		fmt.Printf("[Accept #%d] novo socket criado:\n", connID)
		fmt.Printf("  Conn FD:     %d  <- socket dessa conexão\n", connFD)
		fmt.Printf("  Listener FD: %d  <- continua o mesmo\n", listenerFD)
		fmt.Println()

		go func(c net.Conn) {
			defer c.Close()
			buf := make([]byte, 1024)
			for {
				_, err := c.Read(buf)
				if err != nil {
					return
				}
			}
		}(conn)
	}
}
