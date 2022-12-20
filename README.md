# FactoriGOChatBot

A Discord chatbot for Factorio written in Golang

This bot will work without mods. It supports chat messages and player join/quit. If you want to be able to see more
information on Discord you can use the [companion mod](https://mods.factorio.com/mod/FactoriGOChatBot-companion) OR if
you don't want to use mods you can manually edit your save game!

You can run this bot in Windows, Unix or [Docker](https://hub.docker.com/r/mattie112/factorigo-chat-bot )!

## Requirements / set-up discord bot

### Create a bot
You will need to create your own 'bot' (application).

- Go to: https://discord.com/developers/applications
- Click 'New Application'
- Give the bot a name
- Go to the 'Bot' tab
- Click 'Add Bot'
- Enabled 'Message Content Intent'
- Generate token

### Invite the bot
- Go to the 'General Information' tab and get your application ID
- Go to: https://discord.com/oauth2/authorize?client_id={appid}&permissions=377957370944&scope=bot (Replace `{appid}` with your application ID)
- Then login and add the bot to your server

### Get channel ID

This bot uses a channel ID. In order to see this you need to set your Discord client to 'Developer Mode' (
settings/apperance). With this setting enabled you can right-click on a channe and copy the ID.

## Configuration

### Factorio

You will need to launch your (headless) factorio with the following flags:

```
--rcon-port xx
--rcon-password xx
--console-log /path/to/chatlog (this can also be simply console.log for a file in your servers main directory)
```

RCON is required in order to send messages TO the Factorio server.

If you run your server with a management tool please refer to that documentation. For
example [factorio-server-manager](https://github.com/OpenFactorioServerManager/factorio-server-manager) has a `chat-log`
option (= `--console-log`)

### Bot

This bot uses environment variable to configure. Or an `.env` file in the same directory as the executable.

Here is a list of all variables available:

```
LOG_LEVEL=info (optional) # This is the log level, when submitting a bug please set this to debug
DISCORD_TOKEN=xx # The Discord Authentication token for your bot
DISCORD_CHANNEL_ID=xx # The Discord channel ID 
RCON_IP=xx # The IP of your Factorio server
RCON_PORT=xx # The rcon port
RCON_PASSWORD=xx # The rcon password
FACTORIO_LOG=C:\Users\xx\AppData\Roaming\Factorio\console.log (Unix path also supported) # Path to the chat log (--console-log)
MOD_LOG=C:\Users\xx\AppData\Roaming\Factorio\script-output\factorigo-chat-bot\factorigo-chat-bot.log (Optional) # If you use the companion mod supply the path to that log file here
```
When using docker: don't forget to also mount/bind the log-files to your container.  

## Installation

### Binaries (Windows / Unix)

Simply download the Unix or Windows binary from this repo and run the executeable. You will need to pass some
configuration. You can either create a file called `.env` (keep it in the same directory as the executable) or create a
script that first sets these variables. For example in Windows a `.bat` file cound be created with the following:

```
set LOG_LEVEL=info
set DISCORD_TOKEN=xx
(etc)
factorigo-chat-bot.exe
```

### Docker

Get the image from https://hub.docker.com/r/mattie112/factorigo-chat-bot and run it with the variables as listed above.

Example command:

```
docker run -d --name='factorigo-chat-bot' -e 'LOG_LEVEL'='info' -e 'DISCORD_TOKEN'='xx.xx.xx-xx-xx' -e 'DISCORD_CHANNEL_ID'='xx' -e 'RCON_IP'='192.168.100.xx' -e 'RCON_PORT'='34198' -e 'RCON_PASSWORD'='xx' -e 'FACTORIO_LOG'='/opt/project/factorio.log' -e 'MOD_LOG'='/opt/project/factorigo-chat-bot.log' -v '/mnt/user/appdata/fsm_factorio/console.log':'/opt/project/factorio.log':'rw' -v '/mnt/user/appdata/fsm_factorio/script-output/factorigo-chat-bot/factorigo-chat-bot.log':'/opt/project/factorigo-chat-bot.log':'rw' 'mattie112/factorigo-chat-bot:latest'
```

### Unraid

Do you use Unraid? Let me know then I can see if I can provide a template!

### Build from source

```
Unix:
GOOS=linux GARCH=amd64 CGO_ENABLED=0 go build -ldflags "-X main.VERSION=$(git rev-parse --short HEAD) -X main.BUILDTIME=`date -u +%Y%m%d.%H%M%S`" -o ./bin/factorigo-chat-bot
Windows:
GOOS=windows GARCH=amd64 CGO_ENABLED=0 go build -ldflags "-X main.VERSION=$(git rev-parse --short HEAD) -X main.BUILDTIME=$(date -u +%Y%m%d.%H%M%S)" -o ./bin/factorigo-chat-bot.exe
```

If you don't have a build environment:
Run: `docker-compose run --rm builder` then copy/paste the commands listed above

In order to build a docker image, first run the commands above and then:

- `docker build -t mattie112/factorigo-chat-bot:latest .`
- `docker push mattie112/factorigo-chat-bot:latest`

## Extra data supported:

(With companion mod)

- Research started / finished
- Player deaths
- Rocket launched

## Extra data without mod

If you don't want to use my companion mod you can also manually edit your save file to get the extra data!

- Extract your factorio save/zip file
- Add the contents of [control.lua](https://github.com/Mattie112/FactoriGOChatBot-companion/blob/main/control.lua) to
  the `control.lua` in your save
- Re-zip your save file
