FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ENV CGO_ENABLED=0
RUN go build -trimpath -ldflags="-s -w" -o /out/rmj-card .

FROM alpine:3.22
RUN apk add --no-cache su-exec && addgroup -S app && adduser -S -G app app
WORKDIR /app
COPY --from=build /out/rmj-card /usr/local/bin/rmj-card
COPY --chmod=755 docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN mkdir -p /app/data && chown -R app:app /app
VOLUME ["/app/data"]
ENV PORT=3000
EXPOSE 3000
ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD ["rmj-card"]
