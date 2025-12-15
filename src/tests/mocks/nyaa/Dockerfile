FROM golang:1.24-alpine AS builder

WORKDIR /build

COPY mock_server.go go.mod* ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/mock-server mock_server.go

FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/mock-server .

EXPOSE 8081

ENV PORT=8081
ENV SCENARIO=default

CMD ["./mock-server"]
