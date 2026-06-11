// Estação 04-buffers: client que mede onde Write bloqueia.
//
// Conecta no server e escreve em chunks de 4KB até completar uma quota finita.
// Loga total escrito a cada 100ms. Quando o gráfico para, o send buffer encheu
// — Write está bloqueado esperando flow control liberar espaço. Depois que o
// server acorda e drena, o client termina a quota e fecha a conn, o que faz o
// server enxergar EOF e reportar o total drenado.

//go:build ignore

package main

import (
	"fmt"
	"log"
	"net"
	"time"
)

const (
	chunkSize = 4 * 1024 // 4 KB por Write

	// Quota finita, bem acima do que cabe nos buffers do kernel
	// (rmem.max + wmem.max ~= 10 MB). Garante que o Write trava ANTES de
	// terminar — o freeze que a estação quer mostrar. Sendo finita, o client
	// fecha a conn ao terminar e o server vê EOF (sem isso, io.Copy no server
	// nunca retornaria).
	totalToWrite = 64 * 1024 * 1024 // 64 MB
)

func main() {
	conn, err := net.Dial("tcp", "localhost:8080")
	if err != nil {
		log.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	fmt.Println("=== Estação 04: client write-burst ===")
	fmt.Println("Escrevendo chunks de 4KB. Quando o gráfico congelar = bloqueado.")
	fmt.Println()

	chunk := make([]byte, chunkSize)
	for i := range chunk {
		chunk[i] = 'x'
	}

	totalCh := make(chan int, 1024)
	go func() {
		written := 0
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case n, ok := <-totalCh:
				if !ok {
					return
				}
				written = n
			case <-ticker.C:
				fmt.Printf("\rtotal escrito: %d bytes (%d KB)   ", written, written/1024)
			}
		}
	}()

	total := 0
	start := time.Now()
	for total < totalToWrite {
		n, err := conn.Write(chunk)
		if err != nil {
			fmt.Printf("\nWrite erro após %d bytes em %v: %v\n", total, time.Since(start), err)
			return
		}
		total += n
		select {
		case totalCh <- total:
		default:
		}
	}
	fmt.Printf("\nEscrevi %d bytes em %v. Fechando conn (server vai ver EOF).\n", total, time.Since(start))
	// defer conn.Close() fecha a conn aqui → server destrava do io.Copy com EOF.
}
