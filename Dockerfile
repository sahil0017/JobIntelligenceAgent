FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY contracts ./contracts
COPY intelligence ./intelligence
COPY platform ./platform
RUN CGO_ENABLED=0 go build -o /out/careerpilot ./platform/cmd/careerpilot

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /out/careerpilot /usr/local/bin/careerpilot
ENV CAREERPILOT_DATA_DIR=/data
CMD ["careerpilot"]
