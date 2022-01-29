package main

import (
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/forewing/csgo-rcon"
	"github.com/fsnotify/fsnotify"
	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
	"io"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"
)

var (
	log                 *logrus.Logger
	messagesToDiscord   chan string
	messagesToFactorio  chan string
	readLogFile         chan string
	factorioLogFilePath string
	modLogPath          string
	discordChannelId    string
)

func main() {
	// If we have an .env file -> load it
	if _, err := os.Stat(".env"); err == nil {
		if err := godotenv.Load(); err != nil {
			panic("Could not load .env file")
		}
	}

	log = getLoggerFromConfig(os.Getenv("LOG_LEVEL"), os.Getenv("ENV"))
	log.Infoln("Starting FactoriGO Chat Bot")

	factorioLogFilePath = os.Getenv("FACTORIO_LOG")
	discordChannelId = os.Getenv("DISCORD_CHANNEL_ID")
	modLogPath = os.Getenv("MOD_LOG")

	messagesToDiscord = make(chan string)
	messagesToFactorio = make(chan string)
	readLogFile = make(chan string)

	discord := setUpDiscord()
	rconClient := setUpRCON()
	watcher := setupFileReader()

	go sendDiscordToFactorio(rconClient)
	go readFactorioLogFile()
	go sendMessageToFactorio(discord)

	// Keep running until getting exit signal
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	// Cleanup
	_ = discord.Close()
	_ = watcher.Close()
}

func readFactorioLogFile() {
	for fileName := range readLogFile {
		log.Debug("Trigger to read Factorio logfile")
		line := getLastLineWithSeek(fileName)
		log.WithFields(logrus.Fields{"line": line}).Debug("Read line from Factorio log")
		messagesToFactorio <- parseAndFormatMessage(line)
	}
}

func parseAndFormatMessage(message string) string {
	var re = regexp.MustCompile(`(?m)\[(\w*)]`)
	messageType := re.FindStringSubmatch(message)
	switch messageType[1] {
	case "FactorioChatBot":
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
		return message
	}
}

// With the companion mod (or manual edit of save game) we can extract extra information!
func parseModLogEntries(message string) string {
	var re = regexp.MustCompile(`(?m) \[(.*):`)
	messageType := re.FindStringSubmatch(message)
	switch messageType[1] {
	case "RESEARCH_STARTED":
		var re = regexp.MustCompile(`(?m):(\S*)]`)
		match := re.FindStringSubmatch(message)
		return fmt.Sprintf(":microscope: | Research started: `%s`", match[1])
	default:
		return "whoopsie 2"
	}
}

func sendMessageToFactorio(discord *discordgo.Session) {
	for message := range messagesToFactorio {
		_, err := discord.ChannelMessageSend(discordChannelId, message)
		if err != nil {
			log.WithError(err).WithFields(logrus.Fields{"message": message}).Error("Failed to post message to Discord")
		}
	}
}

func setupFileReader() *fsnotify.Watcher {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				log.WithField("eventName", event).Debug("event")
				if event.Op&fsnotify.Write == fsnotify.Write {
					log.WithField("eventName", event.Name).Debug("modified file")
					readLogFile <- event.Name
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Println("error:", err)
				log.WithError(err).Error("Unable to watch Factorio log file for changes")
			}
		}
	}()

	err = watcher.Add(factorioLogFilePath)
	if err != nil {
		log.Fatal(err)
	}

	if modLogPath != "" {
		err = watcher.Add(modLogPath)
		if err != nil {
			log.Fatal(err)
		}
	}

	return watcher
}

func sendDiscordToFactorio(rconClient *rcon.Client) {
	log.Debugf("Setting up message handler")
	for message := range messagesToDiscord {
		message = strings.Replace(message, "'", "\\'", -1)
		cmd := "/silent-command game.print('[color=#7289DA][Discord]" + message + "[/color]')"
		log.WithFields(logrus.Fields{"cmd": cmd}).Debug("Sending command to Factorio (through RCON)")
		_, err := rconClient.Execute(cmd)
		if err != nil {
			log.WithError(err).WithFields(logrus.Fields{"cmd": cmd}).Error("Unable to send message to Factorio")
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

func onReceiveDiscordMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore all messagesToDiscord created by the bot itself
	if m.Author.ID == s.State.User.ID {
		return
	}

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

	// If the message is "pong" reply with "Ping!"
	if m.Content == "pong" {
		_, err := s.ChannelMessageSend(m.ChannelID, "Ping!")
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
	messagesToDiscord <- fmt.Sprintf("[%s]: %s", nick, m.Content)
}

func getLoggerFromConfig(logLevel, env string) *logrus.Logger {
	logLevel = strings.ToLower(logLevel)
	env = strings.ToLower(env)
	log := logrus.New()
	log.SetFormatter(&logrus.TextFormatter{ForceQuote: true, TimestampFormat: time.RFC3339Nano})

	switch logLevel {
	case "debug":
		log.Level = logrus.DebugLevel
	case "info":
		log.Level = logrus.InfoLevel
	case "warning":
		log.Level = logrus.WarnLevel
	case "fatal":
		log.Level = logrus.FatalLevel
	default:
		log.Level = logrus.InfoLevel
	}
	return log
}

func getLastLineWithSeek(filepath string) string {
	fileHandle, err := os.Open(filepath)

	if err != nil {
		panic("Cannot open file")
		os.Exit(1)
	}
	defer fileHandle.Close()

	line := ""
	var cursor int64 = 0
	stat, _ := fileHandle.Stat()
	filesize := stat.Size()
	for {
		cursor -= 1
		fileHandle.Seek(cursor, io.SeekEnd)

		char := make([]byte, 1)
		fileHandle.Read(char)

		if cursor != -1 && (char[0] == 10 || char[0] == 13) { // stop if we find a line
			break
		}

		line = fmt.Sprintf("%s%s", string(char), line) // there is more efficient way

		if cursor == -filesize { // stop if we are at the begining
			break
		}
	}

	return line
}
