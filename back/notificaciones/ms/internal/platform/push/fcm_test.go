package push

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWebpushLink(t *testing.T) {
	assert.Nil(t, (&FCM{}).webpush("/visitors/1"), "sin WEB_APP_URL no hay link")

	f := &FCM{webURL: "https://app.example.com"}
	assert.Equal(t, "https://app.example.com/visitors/1", f.webpush("/visitors/1").FCMOptions.Link)
	assert.Equal(t, "https://app.example.com/notificaciones", f.webpush("").FCMOptions.Link)
	assert.Equal(t, "https://app.example.com/notificaciones", f.webpush("https://evil.example").FCMOptions.Link)
}
