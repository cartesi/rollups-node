FROM golang:alpine AS builder
WORKDIR /app
COPY . /app
RUN go build

FROM gcr.io/distroless/static-debian12
COPY --from=builder /app/node-pre-start /usr/bin/
CMD [ "/usr/bin/node-pre-start" ]
