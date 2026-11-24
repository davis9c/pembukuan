FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /pembukuan .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
ENV TZ=Asia/Jakarta
COPY --from=builder /pembukuan /pembukuan
RUN mkdir -p /data/uploads
VOLUME /data
EXPOSE 8080
ENV DB_PATH=/data/pembukuan.db
ENV UPLOADS_DIR=/data/uploads
ENV ADMIN_USER=admin
ENV ADMIN_PASS=Admin1234
CMD ["/pembukuan"]