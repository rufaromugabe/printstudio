FROM golang:1.25-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -o /api .
FROM scratch
COPY --from=build /api /api
EXPOSE 8080
ENTRYPOINT ["/api"]
