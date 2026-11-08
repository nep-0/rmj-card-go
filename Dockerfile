FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ENV CGO_ENABLED=0
RUN go build -trimpath -ldflags="-s -w" -o /out/rmj-card .

FROM alpine:3.22
RUN addgroup -S app && adduser -S -G app app
WORKDIR /app
COPY --from=build /out/rmj-card /usr/local/bin/rmj-card
RUN mkdir -p /app/data && chown -R app:app /app
VOLUME ["/app/data"]
USER app
ENV PORT=3000
EXPOSE 3000
ENTRYPOINT ["/usr/local/bin/rmj-card"]
