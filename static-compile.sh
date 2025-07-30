CC=x86_64-linux-musl-gcc \
CGO_ENABLED=1 \
CGO_CFLAGS="-static-libgcc"
CGO_LDFLAGS="-static-libgcc"
GOOS=linux \
GOARCH=amd64 \
go build -tags='' -o bin/linux-amd64/jamduna .
