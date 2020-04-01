FROM alpine:3.12

RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories
RUN apk --no-cache add ca-certificates

USER nobody

COPY --chown=nobody:nobody /bin/flagger .

ENTRYPOINT ["./flagger"]
