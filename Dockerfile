FROM alpine:3.19
LABEL org.opencontainers.image.title="arrgo" \
      org.opencontainers.image.description="Placeholder container image for the arrgo services."

# Create non-root user for running the container
RUN adduser -S appuser
USER appuser

CMD ["/bin/sh", "-c", "echo 'arrgo container image placeholder - replace with service binary when available.' && sleep 3600"]
