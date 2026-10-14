package push

import (
	"context"
	"fmt"
	"strings"
	"time"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"google.golang.org/api/option"
)

type Message struct {
	Tokens   []string
	Title    string
	Body     string
	Type     string
	Data     map[string]interface{}
	Critical bool
}

type Result struct {
	Failed       []string
	Unregistered []string
}

type Sender interface {
	Send(ctx context.Context, msg Message) (Result, error)
}

type FCM struct {
	client *messaging.Client
	webURL string
}

// NewFCM crea el cliente de FCM. webAppURL (opcional, https://…) es la URL del
// front: al tocar un push en el navegador se abre webAppURL + deep_link.
func NewFCM(ctx context.Context, credentialsFile, projectID, webAppURL string) (*FCM, error) {
	app, err := firebase.NewApp(ctx, &firebase.Config{ProjectID: projectID}, option.WithCredentialsFile(credentialsFile))
	if err != nil {
		return nil, fmt.Errorf("failed to initialize firebase app: %w", err)
	}
	client, err := app.Messaging(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize messaging client: %w", err)
	}
	return &FCM{client: client, webURL: strings.TrimRight(webAppURL, "/")}, nil
}

func (f *FCM) Send(ctx context.Context, msg Message) (Result, error) {
	if len(msg.Tokens) == 0 {
		return Result{}, nil
	}

	data := make(map[string]string, len(msg.Data)+1)
	for k, v := range msg.Data {
		data[k] = fmt.Sprintf("%v", v)
	}
	data["type"] = msg.Type

	ttl := 24 * time.Hour
	androidPriority, notifPriority, channel, apnsPriority := "normal", messaging.PriorityDefault, "resident_updates", "5"
	if msg.Critical {
		androidPriority, notifPriority, channel, apnsPriority = "high", messaging.PriorityHigh, "high_importance_channel", "10"
	}

	resp, err := f.client.SendEachForMulticast(ctx, &messaging.MulticastMessage{
		Tokens:       msg.Tokens,
		Data:         data,
		Notification: &messaging.Notification{Title: msg.Title, Body: msg.Body},
		Android: &messaging.AndroidConfig{
			Priority: androidPriority,
			TTL:      &ttl,
			Notification: &messaging.AndroidNotification{
				ChannelID: channel,
				Priority:  notifPriority,
				Sound:     "default",
			},
		},
		APNS: &messaging.APNSConfig{
			Headers: map[string]string{"apns-priority": apnsPriority},
			Payload: &messaging.APNSPayload{Aps: &messaging.Aps{Sound: "default"}},
		},
		Webpush: f.webpush(data["deep_link"]),
	})
	if err != nil {
		return Result{}, fmt.Errorf("failed to send push: %w", err)
	}

	var res Result
	for i, r := range resp.Responses {
		if r.Error == nil {
			continue
		}
		res.Failed = append(res.Failed, msg.Tokens[i])
		if messaging.IsRegistrationTokenNotRegistered(r.Error) {
			res.Unregistered = append(res.Unregistered, msg.Tokens[i])
		}
	}
	return res, nil
}

// webpush define a dónde lleva el clic en la notificación del navegador. FCM
// solo acepta links https absolutos; sin WEB_APP_URL el clic no navega.
func (f *FCM) webpush(deepLink string) *messaging.WebpushConfig {
	if f.webURL == "" {
		return nil
	}
	if !strings.HasPrefix(deepLink, "/") {
		deepLink = "/notificaciones"
	}
	return &messaging.WebpushConfig{FCMOptions: &messaging.WebpushFCMOptions{Link: f.webURL + deepLink}}
}
