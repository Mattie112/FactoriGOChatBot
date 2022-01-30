FROM alpine:latest
WORKDIR /opt/project
COPY ./bin/factorigo-chat-bot /opt/project/factorigo-chat-bot

ENTRYPOINT ["/opt/project/factorigo-chat-bot"]
