package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

const WHATSAPP_SERVER = "s.whatsapp.net"
const GROUP_SUFFIX = "g.us"

type WhatsappService struct {
	client          *whatsmeow.Client
	eventsListeners []Listener
}

type ListenerParams struct {
	Service *WhatsappService
	Event   interface{}
}

type Listener func(*ListenerParams) error

func NewWhatsappService(client *whatsmeow.Client) *WhatsappService {

	service := &WhatsappService{
		client:          client,
		eventsListeners: make([]Listener, 0),
	}

	return service
}

func (s *WhatsappService) Init() error {

	if err := initWhatsappConn(s.client); err != nil {
		return err
	}

	s.client.AddEventHandler(s.HandleEvents)
	return nil
}

func initWhatsappConn(client *whatsmeow.Client) error {
	if client.Store.ID != nil {
		return client.Connect()
	}

	// No ID stored, new login
	qrChan, _ := client.GetQRChannel(context.Background())
	err := client.Connect()
	if err != nil {
		return err
	}

	// TODO: add logging with logger (yavidor)
	for evt := range qrChan {
		if evt.Event == "code" {
			qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
			fmt.Println("QR code:", evt.Code)
		} else {
			fmt.Println("Login event:", evt.Event)
		}
	}

	return nil
}

func (s *WhatsappService) SendMessage(content, user string) error {
	targetJID := types.NewJID(user, WHATSAPP_SERVER)
	_, err := s.client.SendMessage(context.Background(), targetJID, &waE2E.Message{Conversation: proto.String(content)})
	if err != nil {
		return err
	}
	return nil
}

func (s *WhatsappService) React(sender, chat types.JID, id, emoji string) error {
	reaction := s.client.BuildReaction(chat, sender, id, emoji)
	_, err := s.client.SendMessage(context.Background(), chat, reaction)
	if err != nil {
		return err
	}
	return nil

}

func (s *WhatsappService) OnMessage(listener func(service *WhatsappService, msg *events.Message) error) *WhatsappService {

	l := func(params *ListenerParams) error {
		switch v := params.Event.(type) {
		case *events.Message:
			return listener(s, v)
		}

		return nil
	}

	s.eventsListeners = append(s.eventsListeners, l)
	return s
}

func (s *WhatsappService) createJIDsFromPhones(phones ...string) []types.JID {
	jids := make([]types.JID, 0)
	for _, phone := range phones {
		jid := types.NewJID(phone, WHATSAPP_SERVER)
		jids = append(jids, jid)
	}
	return jids
}

func CombineErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	}

	var nonNilErrors []string
	for _, err := range errs {
		if err != nil {
			nonNilErrors = append(nonNilErrors, err.Error())
		}
	}

	if len(nonNilErrors) == 0 {
		return nil
	}

	combinedMessage := strings.Join(nonNilErrors, "; ")

	return errors.New(combinedMessage)
}

func (s *WhatsappService) HandleEvents(evt interface{}) {
	errors := make([]error, 0)

	for _, listener := range s.eventsListeners {
		err := listener(&ListenerParams{
			Service: s,
			Event:   evt,
		})

		if err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		err := CombineErrors(errors)
		fmt.Println("err", strings.Split(err.Error(), "OOF"))
	}
}
