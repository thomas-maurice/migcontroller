FROM alpine:3.19

RUN apk add --no-cache \
    rclone \
    bash \
    coreutils

# Copy migration script
COPY build/migrator/migrate.sh /usr/local/bin/migrate.sh
RUN chmod +x /usr/local/bin/migrate.sh

# Run as root to handle files with any ownership
ENTRYPOINT ["/usr/local/bin/migrate.sh"]
