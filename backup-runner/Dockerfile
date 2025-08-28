# --- build stage ---
FROM golang:1.23-alpine AS build
RUN apk add --no-cache git
WORKDIR /src

# mod cache
COPY go.mod ./
RUN go mod download

# source + tidy (generates go.sum)
COPY . .
RUN go mod tidy

# build
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /backup-runner .

# --- runtime stage ---
FROM alpine:3.20
RUN apk add --no-cache postgresql16-client aws-cli ca-certificates tzdata
COPY --from=build /backup-runner /usr/local/bin/backup-runner
ENTRYPOINT ["backup-runner"]
