
#FROM golang:1.26.2-alpine3.23 as builder

#RUN echo "🚀 Building Go binary for Linux..." \
#    GOOS=linux GOARCH=amd64 go build -o gossets main.go \
#    echo "✅ Build complete! Binary: ./gossets"

# TODO ...
# -------------------------------
FROM alpine:latest

RUN apk --no-cache add tzdata libc6-compat

WORKDIR /app

COPY --chmod=755 gossets .

# Copy templates
COPY  templates ./templates

# Expose default port
EXPOSE 8080

# Run the binary
CMD ["./gossets"]
