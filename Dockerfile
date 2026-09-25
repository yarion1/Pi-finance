# syntax=docker/dockerfile:1
# Imagem única (API + front + worker + migrações). Construída no próprio Pi (ARM64)
# pelo runner self-hosted; também compila em amd64 para o CI.

FROM node:22-alpine AS front
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM golang:1.26-alpine AS back
WORKDIR /src
ENV CGO_ENABLED=0
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY web/embed.go ./web/embed.go
COPY --from=front /web/dist ./web/dist
ARG VERSAO=dev
RUN go build -trimpath -ldflags "-s -w -X main.versao=${VERSAO}" -o /financas ./cmd/financas

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=back /financas /financas
USER 65532:65532
EXPOSE 3100
HEALTHCHECK --interval=30s --timeout=6s --start-period=20s --retries=3 CMD ["/financas", "saude"]
ENTRYPOINT ["/financas"]
CMD ["serve"]
