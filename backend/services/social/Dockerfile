FROM golang:1.27.1 AS build
WORKDIR /src
COPY go.work go.work
COPY shared shared
COPY services/social services/social
WORKDIR /src/services/social
RUN go build -o /out/social ./cmd/social

FROM gcr.io/distroless/base-debian12
COPY --from=build /out/social /social
ENTRYPOINT ["/social"]