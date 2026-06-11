// Estação 02-tupla: a 4-tupla materializada.
//
// Server aceita N conexões. Pra cada uma, loga LocalAddr e RemoteAddr.
// Você vai ver: mesma porta destino (8080), mas portas de origem diferentes.

package main

import (
	"fmt"
	"log"
	"net"
)

func main() {
	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	fmt.Println("=== Estação 02: 4-tupla ===")
	fmt.Println("Servidor escutando em :8080")
	fmt.Println()
	fmt.Println("De outro shell do server, conecte 2 vezes:")
	fmt.Println("  nc localhost 8080")
	fmt.Println()
	fmt.Println("Do inspector, observe:")
	fmt.Println("  ss -tn")
	fmt.Println("  cat /proc/net/tcp")
	fmt.Println()

	connID := 0
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("accept: %v", err)
			continue
		}
		connID++

		// LocalAddr  = lado do servidor da tupla = (IP_dst da conexão, porta_dst)
		// RemoteAddr = lado do cliente da tupla  = (IP_src da conexão, porta_src)
		fmt.Printf("[conn %d] tupla:\n", connID)
		fmt.Printf("  Local  (server side): %s\n", conn.LocalAddr())
		fmt.Printf("  Remote (client side): %s\n", conn.RemoteAddr())
		fmt.Println()

		// Mantém a conexão aberta sem ler/escrever.
		// Goroutine fica parada — você consegue inspecionar a conexão ESTABLISHED.
		go func(c net.Conn, id int) {
			defer c.Close()
			buf := make([]byte, 1024)
			for {
				_, err := c.Read(buf)
				if err != nil {
					fmt.Printf("[conn %d] fechou: %v\n", id, err)
					return
				}
			}
		}(conn, connID)
	}
}
