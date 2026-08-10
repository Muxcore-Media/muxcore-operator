# syntax=docker/dockerfile:1
FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/muxcore-operator ./cmd

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/muxcore-operator /muxcore-operator
USER nonroot:nonroot
ENTRYPOINT ["/muxcore-operator"]
