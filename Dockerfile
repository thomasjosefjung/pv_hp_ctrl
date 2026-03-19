# Stage 1: Build the application
FROM golang:1.26-alpine AS builder

WORKDIR /app

# Copy go.mod (always) and go.sum (if present)
COPY go.mod .
# COPY go.sum . 
# Download all dependencies. Dependencies will be cached if the go.mod and go.sum files are not changed
# RUN go mod download

# Copy the source code into the container
COPY . .

# Build the Go app
# -o /app/main creates the binary in the /app directory
# CGO_ENABLED=0 is important for creating a static binary that can run in a minimal container
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/main .

# Stage 2: Deploy the application
FROM alpine:latest

WORKDIR /app

# Copy the built binary from the builder stage
COPY --from=builder /app/main .

# Copy the configuration file
COPY config.json .

# Expose port 8080 to the outside world
ENTRYPOINT ["/app/main"]
EXPOSE 8081

# Command to run the executable
CMD ["./main"]
