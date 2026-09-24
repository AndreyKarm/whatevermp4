FROM golang:1.27.1 AS build
WORKDIR /src
COPY go.work go.work
COPY shared shared
COPY services/auth services/auth
WORKDIR /src/services/auth
RUN go build -o /out/auth ./cmd/auth

FROM gcr.io/distroless/base-debian12
COPY --from=build /out/auth /auth
ENTRYPOINT ["/auth"]