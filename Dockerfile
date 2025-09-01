# ---------- Build stage (Debian) ----------
FROM golang:1.22-bookworm AS build
WORKDIR /src
COPY ./backend/ ./
# ถ้าต้องการ CGO/SQLite ให้แน่ใจว่ามี gcc/make
RUN apt-get update && apt-get install -y --no-install-recommends build-essential git \
    && rm -rf /var/lib/apt/lists/*
RUN go mod download
# สร้างไบนารี (เปิด CGO)
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o /out/myapp .

# ---------- Runtime stage (Debian slim) ----------
FROM debian:bookworm-slim
WORKDIR /app
COPY --from=build /out/myapp /app/myapp
# ถ้า runtime ต้องการ ca-certificates (เรียก API ออกเน็ต) แนะนำติดตั้ง
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*
EXPOSE 8000
CMD ["/app/myapp"]
