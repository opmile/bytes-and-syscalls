// Estação 05-states: cenário B — servidor recebe FIN mas NÃO fecha (bug).
//
// Cliente vai fechar primeiro. Servidor lê EOF mas "esquece" defer Close.
// Resultado: lado servidor fica em CLOSE_WAIT até o processo terminar.
// Em produção, isso seria um vazamento de socket por bug — visível em
// `ss -tan | grep CLOSE-WAIT` acumulando.

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

	fmt.Println("=== Estação 05B: servidor com bug de close ===")
	fmt.Println("Cliente vai fechar primeiro. Servidor não chama Close.")
	fmt.Println("Resultado: socket fica em CLOSE_WAIT.")
	fmt.Println("Aguarde cliente...")

	conn, err := listener.Accept()
	if err != nil {
		log.Fatalf("accept: %v", err)
	}

	fmt.Printf("Cliente conectado: %s\n", conn.RemoteAddr())

	// Lê até EOF — quando o cliente fechar, recebemos EOF.
	buf := make([]byte, 1024)
	for {
		n, err := conn.Read(buf)
		if n > 0 {
			fmt.Printf("Recebi %d bytes: %q\n", n, buf[:n])
		}
		if err != nil {
			fmt.Printf("Read terminou: %v\n", err)
			break
		}
	}

	// BUG INTENCIONAL: deveria ter conn.Close() aqui.
	// fmt.Println("Esquecendo de fechar (bug intencional)")
	fmt.Println("\nServidor recebeu FIN do cliente mas NÃO fechou seu lado.")
	fmt.Println("Do inspector, observe AGORA:")
	fmt.Println("  ss -tan | grep 8080")
	fmt.Println("\nDeve ver: CLOSE-WAIT no lado :8080.")
	fmt.Println("Sleep 90s pra você inspecionar.")
	fmt.Println("(Em produção, esse socket vazaria até o processo morrer.)")

	// time.Sleep (não select{}): com a única goroutine bloqueada sem como
	// progredir, o deadlock detector do Go mataria o processo antes de você
	// ver o CLOSE_WAIT no ss. O timer evita isso.
	time.Sleep(90 * time.Second)
}
