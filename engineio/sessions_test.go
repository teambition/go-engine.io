package engineio

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestServerSessions(t *testing.T) {
	assert := assert.New(t)
	sessions := newServerSessions()

	assert.Nil(sessions.Get("a"))

	sessions.Set("b", new(serverConn))
	assert.NotNil(sessions.Get("b"))

	assert.Nil(sessions.Get("a"))

	sessions.Set("c", new(serverConn))
	assert.NotNil(sessions.Get("c"))

	sessions.Remove("b")
	assert.Nil(sessions.Get("b"))

}
