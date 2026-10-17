FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/launchgate ./cmd/launchgate

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /out/launchgate /usr/local/bin/launchgate
COPY policies/ /app/policies/
ENV PORT=4020
EXPOSE 4020
ENTRYPOINT ["/usr/local/bin/launchgate"]
CMD ["serve"]
