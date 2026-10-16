FROM golang:1.27.2-alpine@sha256:f92b6ef800e499660581efdabdf25d9d817a9d124eaf900924f0504e7e27e12d AS source
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

FROM source AS verify
RUN sh scripts/verify.sh

FROM source AS build
RUN CGO_ENABLED=0 go build -trimpath -o /out/api ./cmd/api

FROM source AS goose-build
RUN CGO_ENABLED=0 go install -tags='no_clickhouse no_libsql no_mssql no_mysql no_sqlite3 no_vertica no_ydb' github.com/pressly/goose/v3/cmd/goose@v3.29.0

FROM alpine:3.24.0@sha256:a2d49ea686c2adfe3c992e47dc3b5e7fa6e6b5055609400dc2acaeb241c829f4 AS migrate
WORKDIR /app
COPY --from=goose-build /go/bin/goose /usr/local/bin/goose
COPY migrations ./migrations
USER 65532:65532
ENTRYPOINT ["goose", "-dir", "/app/migrations"]
CMD ["up"]

FROM alpine:3.24.0@sha256:a2d49ea686c2adfe3c992e47dc3b5e7fa6e6b5055609400dc2acaeb241c829f4 AS api
WORKDIR /app
COPY --from=build /out/api /app/api
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/app/api"]
