FROM golang:1.26 AS build

WORKDIR /builder

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/windscribe-ephemeral-port-torrent .

FROM gcr.io/distroless/static-debian13

COPY --from=build /out/windscribe-ephemeral-port-torrent /windscribe-ephemeral-port-torrent

ENTRYPOINT [ "/windscribe-ephemeral-port-torrent" ]