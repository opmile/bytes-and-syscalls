// Estação 06-mapping: parser /proc/net/tcp + lookup inode → PID.
//
// O que `ss -p` faz por baixo, implementado à mão.
//
// Algoritmo:
//   1. Lê /proc/net/tcp → coleta (local_addr, remote_addr, state, inode) por linha.
//   2. Varre /proc/<pid>/fd/* → pra cada symlink, lê o destino.
//      Se for "socket:[<inode>]", guarda mapping inode → pid.
//   3. Junta tudo: imprime tabela com PID dono de cada socket.

package main

import (
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// hex de tcp state → nome legível (do create-a-socker-in-go.md).
var tcpStates = map[string]string{
	"01": "ESTABLISHED", "02": "SYN_SENT", "03": "SYN_RECV",
	"04": "FIN_WAIT1", "05": "FIN_WAIT2", "06": "TIME_WAIT",
	"07": "CLOSE", "08": "CLOSE_WAIT", "09": "LAST_ACK",
	"0A": "LISTEN", "0B": "CLOSING",
}

type socketEntry struct {
	local  string
	remote string
	state  string
	inode  string
	pid    int    // -1 se não achou
	cmd    string // nome do processo
}

// "0100007F:1F90" → "127.0.0.1:8080".
// IPv4 vem em hex little-endian; porta em hex big-endian.
func parseAddr(s string) string {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return s
	}
	ipBytes, err := hex.DecodeString(parts[0])
	if err != nil || len(ipBytes) != 4 {
		return s
	}
	ip := net.IPv4(ipBytes[3], ipBytes[2], ipBytes[1], ipBytes[0])
	port, _ := strconv.ParseInt(parts[1], 16, 32)
	return fmt.Sprintf("%s:%d", ip, port)
}

func parseProcNetTCP() ([]socketEntry, error) {
	data, err := os.ReadFile("/proc/net/tcp")
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	var out []socketEntry
	// Pula header (linha 0).
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}
		out = append(out, socketEntry{
			local:  parseAddr(fields[1]),
			remote: parseAddr(fields[2]),
			state:  tcpStates[fields[3]],
			inode:  fields[9],
			pid:    -1,
		})
	}
	return out, nil
}

// Varre /proc/<pid>/fd/* e devolve mapping inode → (pid, comm).
func buildInodeIndex() map[string]struct {
	pid int
	cmd string
} {
	index := map[string]struct {
		pid int
		cmd string
	}{}

	socketRE := regexp.MustCompile(`^socket:\[(\d+)\]$`)

	procDirs, _ := filepath.Glob("/proc/[0-9]*")
	for _, dir := range procDirs {
		pidStr := filepath.Base(dir)
		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			continue
		}

		fdDir := filepath.Join(dir, "fd")
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			// pode falhar por permissão se não for o dono — ignora.
			continue
		}

		// Lê comm uma vez por processo.
		comm := ""
		if b, err := os.ReadFile(filepath.Join(dir, "comm")); err == nil {
			comm = strings.TrimSpace(string(b))
		}

		for _, fd := range fds {
			target, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
			if err != nil {
				continue
			}
			m := socketRE.FindStringSubmatch(target)
			if m == nil {
				continue
			}
			inode := m[1]
			index[inode] = struct {
				pid int
				cmd string
			}{pid: pid, cmd: comm}
		}
	}
	return index
}

func main() {
	entries, err := parseProcNetTCP()
	if err != nil {
		fmt.Fprintf(os.Stderr, "erro lendo /proc/net/tcp: %v\n", err)
		os.Exit(1)
	}

	index := buildInodeIndex()

	for i := range entries {
		if owner, ok := index[entries[i].inode]; ok {
			entries[i].pid = owner.pid
			entries[i].cmd = owner.cmd
		}
	}

	fmt.Printf("%-22s %-22s %-12s %-8s %-6s %s\n",
		"LOCAL", "REMOTE", "STATE", "INODE", "PID", "COMM")
	for _, e := range entries {
		pidStr := "-"
		cmdStr := "-"
		if e.pid != -1 {
			pidStr = strconv.Itoa(e.pid)
			cmdStr = e.cmd
		}
		fmt.Printf("%-22s %-22s %-12s %-8s %-6s %s\n",
			e.local, e.remote, e.state, e.inode, pidStr, cmdStr)
	}
}
