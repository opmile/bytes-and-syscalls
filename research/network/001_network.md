## Como a internet funciona

A internet é uma rede global de dispositivos que se comunicam através de protocolos em camadas. Cada camada resolve um problema específico.

---

**Camadas (modelo TCP/IP simplificado)**

**Física / Link** — transmissão de bits por cabo, fibra ou rádio. Ethernet, Wi-Fi operam aqui.

**IP (Internet Protocol)** — endereçamento e roteamento. Cada dispositivo tem um IP. Pacotes são enviados de roteador em roteador até o destino. IP é *connectionless* — não garante entrega.

**TCP / UDP** — transporte.
- TCP: confiável, garante ordem e entrega via handshake (SYN → SYN-ACK → ACK). Usado em HTTP, SSH.
- UDP: sem garantia, mais rápido. Usado em DNS, streaming, jogos.

**HTTP / HTTPS** — camada de aplicação. Define como cliente e servidor trocam recursos (request/response). HTTPS adiciona TLS por cima do TCP para criptografia.

---

**DNS (Domain Name System)**

Traduz nomes (`google.com`) para IPs (`142.250.x.x`). É uma hierarquia distribuída: root servers → TLD servers (`.com`) → authoritative servers. Sem DNS você precisaria memorizar IPs.

---

**Fluxo de uma requisição HTTP**

```
você digita google.com
  → DNS resolve → IP
  → TCP handshake com o servidor
  → TLS handshake (HTTPS)
  → HTTP GET /
  → servidor responde HTML
  → browser renderiza
```

---

**Resumo dos protocolos**

| Protocolo | Camada | Função |
|-----------|--------|--------|
| IP | Rede | Endereçamento e roteamento |
| TCP | Transporte | Entrega confiável |
| UDP | Transporte | Entrega rápida sem garantia |
| DNS | Aplicação | Nome → IP |
| HTTP/HTTPS | Aplicação | Transferência de recursos web |
| TLS | (entre TCP e HTTP) | Criptografia |

---

Se quiser /doc pra registrar isso, é só chamar.