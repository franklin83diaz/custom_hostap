
arm64

```
sudo snap install zig --edge --classic

GOOS=linux GOARCH=arm64 CGO_ENABLED=1 \
CC="zig cc -target aarch64-linux-musl" \
go build -v \
  -ldflags "-s -w -linkmode external -extldflags '-static'" \
  -o wifi-go main.go

 ``` 