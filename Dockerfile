FROM golang:1.13 as builder

WORKDIR /tmp/build
ADD . .
RUN GOOS=linux go build -mod=vendor -ldflags="-s -w"

# ---

FROM busybox:glibc
COPY --from=builder /etc/ssl/certs /etc/ssl/certs
COPY --from=builder /tmp/build/prometheus-acls /usr/local/bin/prometheus-acls
CMD /usr/local/bin/prometheus-acls

