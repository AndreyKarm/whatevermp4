FROM golang:1.27.1 AS build
WORKDIR /src
COPY go.work go.work
COPY shared shared
COPY services/gateway services/gateway
WORKDIR /src/services/gateway
RUN go build -o /out/gateway ./cmd/gateway

FROM gcr.io/distroless/base-debian12
COPY --from=build /out/gateway /gateway
ENTRYPOINT ["/gateway"]