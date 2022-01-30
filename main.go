package main

import (
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/forewing/csgo-rcon"
	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"
)

var (
	log                *logrus.Logger
	messagesToDiscord  chan string
	messagesToFactorio chan string
	readLogFile        chan string
	discordChannelId   string
	// VERSION These can be injected at build time -ldflags "-InputArgs main.VERSION=dev main.BUILD_TIME=201610251410"
	VERSION = "Undefined"
	// BUILDTIME These can be injected at build time -ldflags "-InputArgs main.VERSION=dev main.BUILD_TIME=201610251410"
	BUILDTIME = "Undefined"
)

func main() {
	// If we have an .env file -> load it
	if _, err := os.Stat(".env"); err == nil {
		if err := godotenv.Load(); err != nil {
			panic("Could not load .env file")
		}
	}

	log = getLoggerFromConfig(os.Getenv("LOG_LEVEL"))
	log.Infof("Starting FactoriGO Chat Bot %s - %s", VERSION, BUILDTIME)
	checkRequiredEnvVariables()

	discordChannelId = os.Getenv("DISCORD_CHANNEL_ID")

	messagesToDiscord = make(chan string)
	messagesToFactorio = make(chan string)
	readLogFile = make(chan string)

	discord := setUpDiscord()
	rconClient := setUpRCON()

	//Setup file watcher
	var paths []string
	paths = append(paths, os.Getenv("FACTORIO_LOG"))
	if os.Getenv("MOD_LOG") != "" {
		paths = append(paths, os.Getenv("MOD_LOG"))
	}
	watcher := setupFileWatcher(paths)

	// Start functions that handle the dataflow
	go sendMessageToFactorio(rconClient)
	go readFactorioLogFile()
	go sendMessageToDiscord(discord)

	// Keep running until getting exit signal
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	// Cleanup
	_ = discord.Close()
	_ = watcher.Close()
}

// Parse the message and format it in a way for Discord
func parseAndFormatMessage(message string) string {
	var re = regexp.MustCompile(`(?m)\[(\w*)]`)
	messageType := re.FindStringSubmatch(message)

	if len(messageType) < 2 {
		return ""
	}

	switch messageType[1] {
	case "FactoriGOChatBot":
		// Extracted to keep function small
		return parseModLogEntries(message)
	case "CHAT":
		var re = regexp.MustCompile(`(?m)] (.*): (.*)`)
		match := re.FindStringSubmatch(message)
		return fmt.Sprintf(":speech_left: | `%s`: %s", match[1], match[2])
	case "JOIN":
		var re = regexp.MustCompile(`(?m)] (\w*)`)
		match := re.FindStringSubmatch(message)
		return fmt.Sprintf(":green_circle: | `%s` joined the game!", match[1])
	case "LEAVE":
		var re = regexp.MustCompile(`(?m)] (\w*)`)
		match := re.FindStringSubmatch(message)
		return fmt.Sprintf(":red_circle: | `%s` left the game!", match[1])
	default:
		log.WithField("message", message).Debug("Could not parse message from Factorio, ignoring")
		return ""
	}
}

// With the companion mod (or manual edit of save game) we can extract extra information!
func parseModLogEntries(message string) string {
	var re = regexp.MustCompile(`(?mU) \[(.*):`)
	messageType := re.FindStringSubmatch(message)
	switch messageType[1] {
	case "RESEARCH_STARTED":
		var re = regexp.MustCompile(`(?m):(\S*)]`)
		match := re.FindStringSubmatch(message)
		return fmt.Sprintf(":microscope: | Research started: `%s`", match[1])
	case "RESEARCH_FINISHED":
		var re = regexp.MustCompile(`(?m):(\S*)]`)
		match := re.FindStringSubmatch(message)
		return fmt.Sprintf(":microscope: | Research finished: `%s`", match[1])
	case "PLAYER_DIED":
		var re = regexp.MustCompile(`(?m):(\S*)]`)
		match := re.FindStringSubmatch(message)
		return fmt.Sprintf(":skull: | Player died: `%s`", match[1])
	default:
		log.WithField("message", message).Debug("Could not parse message from mod, ignoring")
		return ""
	}
}

func sendMessageToDiscord(discord *discordgo.Session) {
	for message := range messagesToDiscord {
		_, err := discord.ChannelMessageSend(discordChannelId, message)
		if err != nil {
			log.WithError(err).WithFields(logrus.Fields{"message": message}).Error("Failed to post message to Discord")
		}
	}
}

func sendMessageToFactorio(rconClient *rcon.Client) {
	log.Debugf("Setting up message handler")
	for message := range messagesToFactorio {
		message = strings.Replace(message, "'", "\\'", -1)
		cmd := "/silent-command game.print('[color=#7289DA][Discord]" + message + "[/color]')"
		log.WithFields(logrus.Fields{"cmd": cmd}).Debug("Sending command to Factorio (through RCON)")
		_, err := rconClient.Execute(cmd)
		if err != nil {
			log.WithError(err).WithFields(logrus.Fields{"cmd": cmd}).Error("Unable to send message to Factorio")
		}
	}
}

func onReceiveDiscordMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore all messagesToDiscord created by the bot itself
	if m.Author.ID == s.State.User.ID {
		return
	}

	// Only listen on our Factorio channel
	if m.ChannelID != discordChannelId {
		return
	}

	log.WithFields(logrus.Fields{"message": m.Content, "author": m.Author}).Debug("Received message on Discord")

	// If the message is "ping" reply with "Pong!"
	if m.Content == "ping" {
		_, err := s.ChannelMessageSend(m.ChannelID, "Pong!")
		if err != nil {
			log.WithError(err).WithFields(logrus.Fields{"message": m.Content}).Error("Failed to send message to Discord")
		}
	}

	// Send message away!
	log.WithFields(logrus.Fields{"message": m.Content}).Debugf("Sending Discord message to output channel")
	nick := m.Member.Nick
	if nick == "" {
		nick = m.Author.Username
	}
	messagesToFactorio <- fmt.Sprintf("[%s]: %s", nick, m.Content)
}

// Read the last line of a file and puts the parsed message on our output channel
func readFactorioLogFile() {
	for fileName := range readLogFile {
		log.Debug("Trigger to read Factorio logfile")
		line := getLastLineWithSeek(fileName)
		log.WithFields(logrus.Fields{"line": line}).Debug("Read line from Factorio log")
		message := parseAndFormatMessage(line)
		if message != "" {
			messagesToDiscord <- message
		}
	}
}

func setUpRCON() *rcon.Client {
	rconIp := os.Getenv("RCON_IP")
	rconPort := os.Getenv("RCON_PORT")
	rconPassword := os.Getenv("RCON_PASSWORD")
	rconClient := rcon.New(rconIp+":"+rconPort, rconPassword, time.Second*2)
	return rconClient
}

func setUpDiscord() *discordgo.Session {
	discordToken := os.Getenv("DISCORD_TOKEN")

	discord, err := discordgo.New("Bot " + discordToken)
	if err != nil {
		log.WithFields(logrus.Fields{"err": err, "token": discordToken}).Panic("Could register bot with Discord")
	}

	// Listen to incoming messagesToDiscord from Discord
	discord.AddHandler(onReceiveDiscordMessage)
	discord.Identify.Intents = discordgo.IntentsGuildMessages

	// Open socket to Discord
	if discord.Open() != nil {
		log.WithFields(logrus.Fields{"err": err}).Panic("Cannot open socket to Discord")
	}

	log.Infoln("Bot registered by Discord and is now listening for messagesToDiscord")
	return discord
}

func checkRequiredEnvVariables() {
	vars := []string{"DISCORD_TOKEN", "DISCORD_CHANNEL_ID", "RCON_IP", "RCON_PORT", "RCON_PASSWORD", "FACTORIO_LOG"}
	for _, envVar := range vars {
		if os.Getenv(envVar) == "" {
			log.WithField("envVar", envVar).Fatal("Could not find required ENV VAR")
		}
	}
}
