# Helper image that copies a project into the staging volume. Built by api on first use.
FROM alpine:3.22
RUN apk add --no-cache git tar grep findutils
COPY copy-project.sh /usr/local/bin/copy-project
RUN chmod +x /usr/local/bin/copy-project
ENTRYPOINT ["/usr/local/bin/copy-project"]
