# ---------- Build stage (Debian) ----------
FROM golang:1.22-bookworm AS build
WORKDIR /src
ARG APP_VERSION=2.0.1
ARG APP_GIT_COMMIT=unknown
ARG APP_GIT_BRANCH=main
ARG APP_DEPLOY_DATE=
ARG APP_SOURCE=server
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
COPY --from=build /src/release-notes.json /app/release-notes.json
ARG APP_VERSION=2.0.1
ARG APP_GIT_COMMIT=unknown
ARG APP_GIT_BRANCH=main
ARG APP_DEPLOY_DATE=
ARG APP_SOURCE=server
ENV APP_VERSION=${APP_VERSION}
ENV APP_GIT_COMMIT=${APP_GIT_COMMIT}
ENV APP_GIT_BRANCH=${APP_GIT_BRANCH}
ENV APP_DEPLOY_DATE=${APP_DEPLOY_DATE}
ENV APP_SOURCE=${APP_SOURCE}
ENV APP_RELEASE_NOTES_PATH=/app/release-notes.json
# ถ้า runtime ต้องการ ca-certificates (เรียก API ออกเน็ต) แนะนำติดตั้ง
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*
EXPOSE 8000
CMD ["/app/myapp"]
