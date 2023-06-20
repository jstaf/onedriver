package graph

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSha1HashReader(t *testing.T) {
	content := []byte("this is some text to hash")
	expected := SHA1Hash(&content)

	reader := bytes.NewReader(content)
	actual := SHA1HashStream(reader)
	assert.Equal(t, expected, actual)
}

func TestQuickXORHashReader(t *testing.T) {
	content := []byte("this is some text to hash")
	expected := QuickXORHash(&content)

	reader := bytes.NewReader(content)
	actual := QuickXORHashStream(reader)
	assert.Equal(t, expected, actual)
}
