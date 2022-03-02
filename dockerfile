# syntax=docker/dockerfile:1
FROM --platform=linux/amd64 alpine:3.15
RUN apk --no-cache add ca-certificates tini
COPY http-s3 /usr/local/bin
ENTRYPOINT ["tini", "--"]
CMD ["http-s3"]
EXPOSE 3000