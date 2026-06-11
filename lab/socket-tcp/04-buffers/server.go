// Estação 04-buffers: server slow-reader.
//
// Aceita conexão, mas dorme 15s antes de chamar Read.
// Resultado: receive buffer enche, TCP flow control freia o sender,
// send buffer do cliente enche, e o cliente bloqueia no Write.

package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"time"
)

const (
	drainChunk = 4 * 1024 * 1024 // lê ~4MB por pulso (esvazia o recv buffer de uma vez)
	drainPause = 1 * time.Second // pausa entre pulsos → buffer reenche → dá pra ver oscilar
)

func main() {
	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	fmt.Println("=== Estação 04: server slow-reader ===")
	fmt.Println("Aceita conexão, dorme 15s, depois drena tudo.")
	fmt.Println("Aguarde cliente conectar...")

	conn, err := listener.Accept()
	if err != nil {
		log.Fatalf("accept: %v", err)
	}
	defer conn.Close()

	fmt.Printf("Cliente conectado: %s\n", conn.RemoteAddr())
	fmt.Println("Dormindo 15s antes do primeiro Read...")
	fmt.Println("→ enquanto isso, do INSPECTOR rode:")
	fmt.Println("    watch -n 0.5 'ss -tn | grep 8080'")
	fmt.Println("  observe Recv-Q crescer e parar quando o buffer encher.")

	time.Sleep(15 * time.Second)

	fmt.Println("\nAcordei. Drenando em PULSOS (lê 4MB, pausa 1s)...")
	fmt.Println("→ no inspector, veja Recv-Q despencar e reencher a cada pulso.")
	start := time.Now()

	buf := make([]byte, drainChunk)
	var total int64
	for {
		n, err := io.ReadFull(conn, buf)
		total += int64(n)
		if err != nil {
			// io.EOF (fim exato) ou io.ErrUnexpectedEOF (último pulso parcial) = client fechou.
			fmt.Printf("\nDrenei %d bytes em %v. err=%v\n", total, time.Since(start), err)
			break
		}
		fmt.Printf("\rdrenado: %d bytes (%d MB)   ", total, total/1024/1024)
		time.Sleep(drainPause)
	}
}
