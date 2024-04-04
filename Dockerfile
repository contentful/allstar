# Use the official Go image as the base image
FROM golang:1.22 AS builder

# Set the working directory
WORKDIR /ko-app

# Copy the Go application source code
COPY . .

# Tidy em packages
RUN go mod tidy

# Build the Go application
RUN CGO_ENABLED=0 go build -o allstar ./cmd/allstar

# Use a minimal base image to reduce the image size
FROM gcr.io/distroless/base-debian10

# Copy the binary from the builder stage to the final image
COPY --from=builder /ko-app/allstar /ko-app/allstar

# Set the entry point for the final image
ENTRYPOINT ["/ko-app/allstar"]
