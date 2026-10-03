# Helper image that inspects a project and copies it into the staging volume. Built by api on first use.
FROM alpine:3.22
RUN apk add --no-cache git tar grep findutils coreutils sed
COPY copy-project.sh /usr/local/bin/copy-project
COPY inspect-project.sh /usr/local/bin/inspect-project
COPY list-folders.sh /usr/local/bin/list-folders
RUN chmod +x /usr/local/bin/copy-project /usr/local/bin/inspect-project /usr/local/bin/list-folders
ENTRYPOINT ["/usr/local/bin/copy-project"]
