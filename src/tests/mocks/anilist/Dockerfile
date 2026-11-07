FROM golang:1.27-alpine AS builder

WORKDIR /build

COPY mock_server.go go.mod* ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/mock-server mock_server.go

FROM alpine:3.24

WORKDIR /app

COPY --from=builder /app/mock-server .

EXPOSE 8080

ENV PORT=8080
ENV SCENARIO=default

CMD ["./mock-server"]
