FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/launchgate ./cmd/launchgate

FROM alpine:3.20
RUN apk add --no-cache ca-certificates \
    && adduser -D -H -u 10001 launchgate
WORKDIR /app
COPY --from=builder /out/launchgate /usr/local/bin/launchgate
COPY policies/ /app/policies/
ENV PORT=4020
EXPOSE 4020
HEALTHCHECK --interval=15s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -qO- http://127.0.0.1:4020/health || exit 1
USER launchgate
ENTRYPOINT ["/usr/local/bin/launchgate"]
CMD ["serve"]
