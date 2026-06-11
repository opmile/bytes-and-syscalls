// Estação 05-states: cenário A — servidor faz active close.
//
// Servidor aceita conexão, lê o que o cliente mandar, depois FECHA primeiro.
// Resultado: lado servidor entra em TIME_WAIT por ~60s.

//go:build ignore

package main

import (
	"fmt"
	"log"
	"net"
	"time"
)

func main() {
	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	fmt.Println("=== Estação 05A: servidor active close ===")
	fmt.Println("Vai fechar primeiro → entra em TIME_WAIT.")
	fmt.Println("Aguarde cliente...")

	conn, err := listener.Accept()
	if err != nil {
		log.Fatalf("accept: %v", err)
	}

	fmt.Printf("Cliente conectado: %s\n", conn.RemoteAddr())

	// Lê o que vier (cliente vai mandar e desconectar com EOF — mas vamos
	// fechar antes dele).
	buf := make([]byte, 1024)
	n, _ := conn.Read(buf)
	if n > 0 {
		fmt.Printf("Recebi %d bytes: %q\n", n, buf[:n])
	}

	fmt.Println("\nFechando o lado servidor PRIMEIRO (active close)...")
	conn.Close()
	fmt.Println("Fechado. Servidor agora deve estar em TIME_WAIT por ~60s.")
	fmt.Println("\nDo inspector, observe AGORA:")
	fmt.Println("  ss -tan | grep 8080")
	fmt.Println("\nDeve ver: TIME-WAIT no lado :8080.")
	fmt.Println("Sleep 90s pra você inspecionar...")

	// Importante: NÃO chamar listener.Close ainda — manter o programa vivo
	// pra você observar TIME_WAIT desaparecer naturalmente.
	// time.Sleep (não select{}): a única goroutine viva ficaria sem como
	// progredir e o deadlock detector do Go mataria o processo antes de você
	// inspecionar. O timer acorda a goroutine, então não conta como deadlock.
	time.Sleep(90 * time.Second)
}
