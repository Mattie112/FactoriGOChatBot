# Pinned so builds are reproducible, Dependabot keeps it up to date
FROM alpine:3.22
WORKDIR /opt/project
COPY ./bin/factorigo-chat-bot /opt/project/factorigo-chat-bot

ENTRYPOINT ["/opt/project/factorigo-chat-bot"]
