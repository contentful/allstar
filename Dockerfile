FROM golang:1.17

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/. ./cmd
COPY pkg/. ./pkg
RUN ls -al 
RUN CGO_ENABLED=0 GOOS=linux go build -o /allstar ./cmd/allstar 

CMD ["/allstar"]