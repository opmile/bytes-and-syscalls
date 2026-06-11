No Linux você lê de `/proc/net/tcp` (IPv4) e `/proc/net/tcp6` (IPv6) — é assim que o `ss` e o `netstat` fazem por baixo. O kernel expõe a tabela de sockets como texto nesses arquivos.

Cada linha tem campos como endereço local, endereço remoto, estado da conexão (em hex: `01` = ESTABLISHED, `0A` = LISTEN, etc.), inode, uid e por aí vai.

Versão mínima em Go:

```go
package main

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

var tcpStates = map[string]string{
	"01": "ESTABLISHED", "02": "SYN_SENT", "03": "SYN_RECV",
	"04": "FIN_WAIT1", "05": "FIN_WAIT2", "06": "TIME_WAIT",
	"07": "CLOSE", "08": "CLOSE_WAIT", "09": "LAST_ACK",
	"0A": "LISTEN", "0B": "CLOSING",
}

// "0100007F:1F90" -> 127.0.0.1:8080
func parseAddr(s string) string {
	parts := strings.Split(s, ":")
	ipBytes, _ := hex.DecodeString(parts[0])
	// little-endian: inverte os bytes
	ip := net.IPv4(ipBytes[3], ipBytes[2], ipBytes[1], ipBytes[0])
	port, _ := strconv.ParseInt(parts[1], 16, 32)
	return fmt.Sprintf("%s:%d", ip, port)
}

func main() {
	f, err := os.Open("/proc/net/tcp")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Scan() // pula header
	fmt.Printf("%-22s %-22s %s\n", "LOCAL", "REMOTE", "STATE")
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		fmt.Printf("%-22s %-22s %s\n",
			parseAddr(fields[1]),
			parseAddr(fields[2]),
			tcpStates[fields[3]],
		)
	}
}
```

Pontos que vale prestar atenção:

O endereço IPv4 vem em hex e em little-endian, daí a inversão dos bytes. Para IPv6 em `/proc/net/tcp6` o formato é parecido mas com 16 bytes e o endianness por palavra de 32 bits — se quiser cobrir os dois, dá pra abstrair.

O campo `inode` (índice ~9) é a chave para mapear o socket de volta a um processo: você varre `/proc/<pid>/fd/*`, lê os symlinks, e procura `socket:[<inode>]`. É exatamente isso que o `ss -p` faz.

Em macOS e Windows não existe `/proc`, então essa abordagem é Linux-only. No macOS o equivalente é via `sysctl` (`net.inet.tcp.pcblist`) e no Windows é a API `GetExtendedTcpTable`.