# ---------- Build stage (Alpine) ----------
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY ./backend/ ./

# ต้องมี sqlite-dev เพื่อ header & libsqlite3
RUN apk add --no-cache build-base git sqlite-dev

RUN go mod download
# ใช้แท็ก libsqlite3 ให้ลิงก์กับ lib ของระบบ (เลี่ยงปัญหา pread64/…)
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -tags "libsqlite3" -o /out/myapp .

# ---------- Runtime (Alpine) ----------
FROM alpine:3.20
WORKDIR /app
# runtime ต้องมี libsqlite3 ให้ไบนารีลิงก์ได้
RUN apk add --no-cache sqlite-libs ca-certificates
COPY --from=build /out/myapp /app/myapp
EXPOSE 8000
CMD ["/app/myapp"]
