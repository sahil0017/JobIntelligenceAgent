FROM node:22-alpine AS console
WORKDIR /src/console
COPY console/package*.json ./
RUN npm ci
COPY console/ ./
RUN npm run build

FROM golang:1.26-alpine AS api
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY contracts ./contracts
COPY intelligence ./intelligence
COPY platform ./platform
RUN CGO_ENABLED=0 go build -o /out/careerpilot ./platform/cmd/careerpilot

FROM alpine:3.20
RUN apk add --no-cache ca-certificates && adduser -D app && mkdir /data && chown app /data
COPY --from=api /out/careerpilot /usr/local/bin/careerpilot
COPY --from=console /src/console/dist /srv/console
USER app
ENV CONSOLE_DIR=/srv/console CAREERPILOT_DATA_DIR=/data
CMD ["careerpilot"]
