FROM golang:1.25 AS builder

WORKDIR /workspace

# Copy addon-framework (for replace directive)
COPY addon-framework/ ./addon-framework/

# Copy addon-framework-basic
WORKDIR /workspace/addon-framework-basic
COPY addon-framework-basic/go.mod addon-framework-basic/go.sum ./
RUN go mod download

COPY addon-framework-basic/ ./

RUN CGO_ENABLED=0 GOOS=linux go build -o addon ./cmd/addon

FROM alpine:3.19

RUN apk --no-cache add ca-certificates

COPY --from=builder /workspace/addon-framework-basic/addon /addon

ENTRYPOINT ["/addon"]
