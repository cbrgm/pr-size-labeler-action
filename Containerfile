FROM golang:1.21.4-alpine3.17 AS build

WORKDIR /pr-size-labeler-action

COPY . ./
RUN apk --no-cache add make git curl ca-certificates && make release

FROM alpine:latest

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /pr-size-labeler-action/bin/pr-size-labeler-action_linux_amd64 /bin/pr-size-labeler-action

ENTRYPOINT [ "/bin/pr-size-labeler-action" ]
