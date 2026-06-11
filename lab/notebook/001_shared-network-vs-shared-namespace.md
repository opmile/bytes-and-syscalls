# Shared Network vs Shared NET Namespace

## Contexto

Lab `socket-tcp/` em `go-socket`. Setup do `docker-compose.yml` usa dois services (`server` e `inspector`) com `network_mode: "service:server"`. Conceito ancorado em `docker/00-introduction/06-objects.md` ("Containers em redes distintas não se enxergam — a menos que você permita") e em `docker/00-introduction/05-underlying-tech.md` (NET namespace).

A frase do material sugere uma única forma de "permitir comunicação". Na prática existem dois cenários distintos — e o lab usa o segundo, não o primeiro.

## Por que

**Cenário A — mesma rede Docker (uso comum):**

Cada container mantém seu próprio NET namespace, IP próprio, interface própria. "Permitir comunicação" = colocar todos numa mesma bridge nomeada.

```bash
docker network create app-net
docker run --network=app-net --name=db postgres
docker run --network=app-net --name=api minha-api
```

API resolve `db:5432` via DNS interno do Docker. Conversam **pela rede** — pacotes TCP saem de um IP, chegam em outro. Compose faz isso por default: services no mesmo arquivo entram numa rede compartilhada e se resolvem pelo `service name`.

**Cenário B — NET namespace compartilhado (lab `socket-tcp`):**

Não é "duas redes se enxergando". É **um único NET namespace** usado por dois containers. Não há comunicação pela rede — eles estão na mesma rede, literalmente.

```yaml
services:
  server:
    build: .
  inspector:
    build: .
    network_mode: "service:server"
```

Consequências:

- Mesma interface `lo`, mesmo IP, mesma tabela de sockets do kernel
- `inspector` lê `/proc/net/tcp` e vê os sockets do `server`
- Comunicação via `localhost:8080`, não via nome de service
- Inspeção do estado de rede do `server` a partir do `inspector` é trivial — porque é o mesmo namespace

**Analogia:**

- Cenário A: dois apartamentos no mesmo prédio, falam pelo interfone (cada um tem endereço próprio, comunicação via rede do prédio)
- Cenário B: duas pessoas no mesmo apartamento, gritam pelo corredor (mesmo espaço, sem rede no meio)

**Por que o lab precisa do Cenário B:**

Objetivo pedagógico é inspecionar sockets do `server` de fora do processo. Se `inspector` tivesse NET namespace próprio (Cenário A), `cat /proc/net/tcp` dentro dele mostraria a tabela de sockets **dele**, vazia em relação ao servidor. Inútil.

Compartilhando o NET namespace, `inspector` enxerga a mesma tabela que o `server` enxerga — materializa o conceito de namespace de `05-underlying-tech.md` (`/proc/net/tcp` é por namespace, não global).

**Equivalência CLI:**

| Compose                              | CLI                              |
|--------------------------------------|----------------------------------|
| `networks:` (default, mesma rede)    | `--network=app-net`              |
| `network_mode: "service:server"`     | `--network=container:server`     |

São flags diferentes do Docker, não atalhos da mesma coisa. Compose só dá ergonomia declarativa — o conceito subjacente muda.

**Conclusão:**

Usar Compose em vez da CLI não é o ponto. O ponto é qual modo de rede está sendo aplicado. O lab usa `network_mode: "service:X"` porque precisa de inspeção do mesmo NET namespace, não comunicação entre namespaces distintos.
