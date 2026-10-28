FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /sg1 ./cmd/sg1

FROM alpine:3.23
RUN apk add --no-cache ca-certificates docker-cli && addgroup -S -g 10001 sg1 && adduser -S -D -H -u 10001 -G sg1 sg1
COPY --from=build /sg1 /usr/local/bin/sg1
USER sg1
EXPOSE 8080
ENTRYPOINT ["sg1"]
CMD ["serve"]
