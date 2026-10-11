# syntax=docker/dockerfile:1

FROM node:24-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/mixtaipes ./cmd/mixtaipes

FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=build /out/mixtaipes /mixtaipes
EXPOSE 8080
ENTRYPOINT ["/mixtaipes"]
