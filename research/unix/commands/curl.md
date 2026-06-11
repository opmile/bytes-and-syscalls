# curl — cliente HTTP de linha de comando

`curl` é a ferramenta default para falar com qualquer coisa HTTP/HTTPS pela
linha de comando. Para engenharia de backend é tão presente quanto `git` —
você vai usar pra debugar API, testar header, verificar TLS, simular request
de cliente, baixar artifact em CI.

## Mental model

`curl URL` baixa o conteúdo da URL e cospe no stdout. Tudo além disso é
camada de configuração: método, header, body, autenticação, redirect,
certificado, output.

## O essencial (90% do uso)

```bash
curl https://api.exemplo.com/users
curl -i https://api.exemplo.com/users           # inclui response headers no output
curl -I https://api.exemplo.com/users           # só headers (HEAD request)
curl -v https://api.exemplo.com/users           # verbose: mostra request, response, TLS
curl -L https://exemplo.com                     # segue redirects (3xx)
curl -o saida.json https://api.exemplo.com/x    # salva em arquivo
curl -O https://exemplo.com/arquivo.zip         # salva com nome do remote
curl -s https://api.exemplo.com/x | jq          # silencia progress bar (importante em pipe)
curl -sS https://api...                         # silencioso mas mostra erro se falhar
curl -f https://api...                          # falha (exit code != 0) em status >= 400
```

`-fsSL` é a combinação canônica para scripts:
```bash
curl -fsSL https://get.docker.com | bash       # falha cedo, segue redirect, silencioso, mostra erro
```

## Métodos, headers, body

```bash
curl -X POST https://api/users
curl -H "Authorization: Bearer $TOKEN" https://api/me
curl -H "Content-Type: application/json" -d '{"name":"mile"}' https://api/users
curl -d @body.json https://api/users           # body de arquivo
curl --data-urlencode "q=cifras de césar" https://api/search
curl -F "file=@foto.png" https://api/upload     # multipart/form-data
curl -u user:pass https://api/...               # basic auth (cuidado: aparece em ps)
curl --cookie "session=abc" https://...         # cookie inline
curl --cookie-jar j.txt --cookie j.txt https:// # mantém cookies entre chamadas
```

`-d` (POST) já assume `Content-Type: application/x-www-form-urlencoded`. Para
JSON, sempre passe `-H "Content-Type: application/json"` explícito.

## Debug — o que `-v` te mostra

```
* Connected to api.exemplo.com (1.2.3.4) port 443
* TLS handshake completed (TLS 1.3, ECDHE-RSA-AES256-GCM-SHA384)
* Server certificate: CN=api.exemplo.com
> GET /users HTTP/2
> Host: api.exemplo.com
> User-Agent: curl/8.4.0
< HTTP/2 200
< content-type: application/json
{...body...}
```

- `*` = informação do curl (DNS, TCP, TLS).
- `>` = request que está enviando.
- `<` = response que está recebendo.

`--trace-ascii -` ou `--trace -` é ainda mais detalhado — mostra payload byte a
byte. Útil quando o body está corrompido e você não confia em `-v`.

## Timing e conectividade

```bash
curl -w "tempo total: %{time_total}s\n" -o /dev/null -s https://...
curl -w "dns=%{time_namelookup} connect=%{time_connect} tls=%{time_appconnect} ttfb=%{time_starttransfer} total=%{time_total}\n" \
     -o /dev/null -s https://api/...
```

Esse `-w` é precioso pra responder "onde está o gargalo" — DNS, TCP, TLS ou
servidor. Coloca num alias.

## TLS — operações que você vai precisar

```bash
curl -k https://...                              # ignora cert inválido (LAB ONLY)
curl --cacert ca.pem https://...                 # CA customizada
curl --cert client.pem --key client.key https:// # mTLS
curl --resolve api.exemplo.com:443:127.0.0.1 https://api.exemplo.com/  # força DNS
curl --tlsv1.3 https://...                       # força versão TLS
curl -I https://exemplo.com 2>&1 | grep -i 'expire\|subject'  # rápido olhada no cert
```

`--resolve` é mágico em debug: testa "esta máquina aqui responde como se fosse
prod" sem mexer em `/etc/hosts`.

## HTTP/2 e HTTP/3

```bash
curl --http2 https://...
curl --http3 https://...        # depende do build do curl ter suporte
curl -I https://... | head -1   # vê qual versão o server negociou ("HTTP/2 200")
```

## Exemplos no contexto do go-socket / engenharia

```bash
# testar handler Go local
curl -v http://localhost:8080/health

# enviar JSON com auth
curl -fsS -H "Authorization: Bearer $T" -H "Content-Type: application/json" \
     -d '{"q":"go"}' http://localhost:8080/search | jq

# medir cold start de uma API
for i in 1 2 3; do
  curl -w "%{time_total}\n" -o /dev/null -s https://api/...
done

# testar que servidor TCP genérico responde HTTP cru (mais bruto que nc)
curl -v --http0.9 http://localhost:8080  # se você só quer ver os bytes
```

## Quando preferir `wget`

`wget` é melhor para:
- Baixar muitos arquivos recursivamente (`-r`).
- Resumir download interrompido (`-c`).
- Mirror de site (`-m`).

`curl` é melhor para:
- Tudo que envolve API, header, debug, TLS, métodos não-GET, scripting.

Em prática, num backend você vai usar `curl` 95% do tempo.

## Alternativas modernas

- **`httpie`** (`http`) — sintaxe humana (`http POST api/users name=mile`), output
  colorido, JSON nativo. Vale instalar pra uso interativo (`brew install httpie`).
  `curl` ainda ganha pra scripts pela ubiquidade.
- **`xh`** — `httpie` reescrito em Rust, mais rápido.
- **`hurl`** — declara request + assertions em arquivo `.hurl`. Bom pra
  smoke-test em CI.

Mas saiba `curl` bem primeiro — é o que está em **toda** máquina Linux/macOS e
em todo Dockerfile minimalista.
