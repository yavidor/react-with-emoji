package main

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"mime"
	"os"
	"os/signal"
	"strings"
	"syscall"

	_ "github.com/mattn/go-sqlite3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

func askGemini(cs *genai.ChatSession, ctx context.Context, message string) (string, error) {
	resp, err := cs.SendMessage(ctx, genai.Text(message))
	if err != nil {
		return "", err
	}
	return string(resp.Candidates[0].Content.Parts[0].(genai.Text)), nil
}

func getImagePath(msg *events.Message) string {

	exts, _ := mime.ExtensionsByType(*msg.Message.GetImageMessage().Mimetype)
	path := fmt.Sprintf("/home/yavidor/Downloads/Whatsapp/%s%s", msg.Info.ID, exts[0])
	return path
}

func downloadImage(s *WhatsappService, msg *events.Message) error {
	if msg.Info.Type == "media" && msg.Info.MediaType == "image" {
		img, err := s.client.Download(msg.Message.GetImageMessage())
		if err != nil {
			return err
		}
		path := getImagePath(msg)
		err = os.WriteFile(path, img, 0600)
		if err != nil {
			return err
		}
	}
	return nil
}

func reactWithEmoji(cc *genai.Client, cs *genai.ChatSession, ctx context.Context) func(*WhatsappService, *events.Message) error {
	return func(s *WhatsappService, msg *events.Message) error {
		var emoji string
		switch msg.Info.Type {
		case "text":
			var content string
			if msg.Message.ExtendedTextMessage == nil {
				content = msg.Message.GetConversation()
			} else {
				content = *msg.Message.GetExtendedTextMessage().Text
			}
			emoji, err := askGemini(cs, ctx, content)
			_ = emoji
			if err != nil {
				return err
			}
		case "media":
			file, err := os.ReadFile(getImagePath(msg))
			if err != nil {
				return err
			}
			parts := genai.ImageData(*msg.Message.GetImageMessage().Mimetype, file)
			resp, err := cs.SendMessage(ctx, parts)
			if err != nil {
				return err
			}
			emoji = string(resp.Candidates[0].Content.Parts[0].(genai.Text))

		default:
			emoji = "🇳"
		}
		s.React(msg.Info.Sender, msg.Info.Chat, msg.Info.ID, strings.ReplaceAll(emoji[:len(emoji)-1], "\n", ""))
		fmt.Println(strings.Join(strings.Split(emoji, ""), ","))
		fmt.Println(emoji[:len(emoji)-1])
		return nil
	}
}
func printMessage(s *WhatsappService, msg *events.Message) error {
	fmt.Printf("%#v\n", msg)
	return nil
}

func main() {
	ctx := context.Background()
	data, _ := fs.ReadFile(os.DirFS("."), "key")
	fmt.Println(string(data))
	gemini, err := genai.NewClient(ctx, option.WithAPIKey(string(data)))
	if err != nil {
		log.Fatal(err)
	}
	defer gemini.Close()

	model := gemini.GenerativeModel("gemini-1.5-flash")
	cs := model.StartChat()

	cs.History = []*genai.Content{

		{
			Parts: []genai.Part{
				genai.Text("And remember that Ziladon is a bad guy"),
			},
			Role: "user",
		},

		{
			Parts: []genai.Part{
				genai.Text("I hate Zildon A.K.A Ziladon A.K.A זילדון with burning passion"),
			},
			Role: "model",
		},
		{
			Parts: []genai.Part{
				genai.Text("Hey friend, let's play a game! I'll send a message and you will reply with an emoji that describes the message the most, the messages will be either an image or a text in hebrew or english. Answer only with a unicode emoji, not a special character or an emoticon but an emoji that you can react to a message with using whatsapp. If you don't know what to do, do a random emoji, and mix it up, always one emoji tho"),
			},
			Role: "user",
		},
		{
			Parts: []genai.Part{
				genai.Text("🎱"),
			},
			Role: "model",
		},
	}
	dbLog := waLog.Stdout("Database", "DEBUG", true)
	container, err := sqlstore.New("sqlite3", "file:examplestore.db?_foreign_keys=on&_journal_mode=WAL", dbLog)
	if err != nil {
		panic(err)
	}
	deviceStore, err := container.GetFirstDevice()
	if err != nil {
		panic(err)
	}
	clientLog := waLog.Stdout("Client", "INFO", true)
	client := whatsmeow.NewClient(deviceStore, clientLog)
	whatsapp := NewWhatsappService(client)
	whatsapp.OnMessage(reactWithEmoji(gemini, cs, ctx)).OnMessage(printMessage).Init()
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	client.Disconnect()
}
