FROM golang:1.27.1 AS build
WORKDIR /src
COPY go.work go.work
COPY shared shared
COPY services/processing services/processing
WORKDIR /src/services/processing
RUN go build -o /out/processing ./cmd/processing

FROM gcr.io/distroless/base-debian12
COPY --from=build /out/processing /processing
ENTRYPOINT ["/processing"]