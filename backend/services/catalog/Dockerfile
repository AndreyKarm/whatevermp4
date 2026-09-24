FROM golang:1.27.1 AS build
WORKDIR /src
COPY go.work go.work
COPY shared shared
COPY services/catalog services/catalog
WORKDIR /src/services/catalog
RUN go build -o /out/catalog ./cmd/catalog

FROM gcr.io/distroless/base-debian12
COPY --from=build /out/catalog /catalog
ENTRYPOINT ["/catalog"]