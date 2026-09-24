FROM golang:1.27.1 AS build
WORKDIR /src
COPY go.work go.work
COPY shared shared
COPY services/rooms services/rooms
WORKDIR /src/services/rooms
RUN go build -o /out/rooms ./cmd/rooms

FROM gcr.io/distroless/base-debian12
COPY --from=build /out/rooms /rooms
ENTRYPOINT ["/rooms"]