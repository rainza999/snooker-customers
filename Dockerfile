# ---------- Build stage ----------
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY ./backend/ ./
RUN apk add --no-cache build-base git \
 && go mod download \
 && CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o /out/myapp .

# ---------- Runtime stage ----------
FROM alpine:3.20
WORKDIR /app
COPY --from=build /out/myapp /app/myapp
# ถ้าต้องมีไฟล์ static อื่น ๆ ของ backend ก็ COPY เพิ่มที่นี่
EXPOSE 8000
CMD ["/app/myapp"]
