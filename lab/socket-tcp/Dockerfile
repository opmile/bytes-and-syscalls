# Imagem base com Go + ferramentas de inspeção de rede.
# alpine = imagem mínima Linux (~5MB). golang:1.23-alpine = + Go toolchain.
FROM golang:1.23-alpine

# Ferramentas que usaremos nas estações:
# - iproute2 → comando `ss` (substituto moderno do netstat)
# - lsof     → lista FDs abertos por processo
# - procps   → ps, top
# - busybox-extras → telnet, nc com mais opções
# - bash     → shell mais confortável que sh
RUN apk add --no-cache iproute2 lsof procps busybox-extras bash

WORKDIR /app

# O código será montado via volume (docker-compose.yml).
# Não copiamos nada aqui — durante o estudo, editamos no macOS e o container vê na hora.

CMD ["sleep", "infinity"]
