package graph

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func BenchmarkSHA1(b *testing.B) {
	data, _ := os.ReadFile("dmel.fa")
	for i := 0; i < b.N; i++ {
		SHA1Hash(&data)
	}
}

func BenchmarkSHA256(b *testing.B) {
	data, _ := os.ReadFile("dmel.fa")
	for i := 0; i < b.N; i++ {
		SHA256Hash(&data)
	}
}

func BenchmarkQuickXORHash(b *testing.B) {
	data, _ := os.ReadFile("dmel.fa")
	for i := 0; i < b.N; i++ {
		QuickXORHash(&data)
	}
}

func BenchmarkSHA1Stream(b *testing.B) {
	data, _ := os.Open("dmel.fa")
	for i := 0; i < b.N; i++ {
		SHA1HashStream(data)
	}
}

func BenchmarkSHA256Stream(b *testing.B) {
	data, _ := os.Open("dmel.fa")
	for i := 0; i < b.N; i++ {
		SHA256HashStream(data)
	}
}

func BenchmarkQuickXORHashStream(b *testing.B) {
	data, _ := os.Open("dmel.fa")
	for i := 0; i < b.N; i++ {
		QuickXORHashStream(data)
	}
}

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

func TestHashSeekPosition(t *testing.T) {
	tmp, err := os.CreateTemp("", "onedriverHashTest")
	if err != nil {
		t.Error(err)
	}
	content := []byte("some test content")
	io.Copy(tmp, bytes.NewBuffer(content))

	assert.Equal(t, QuickXORHash(&content), QuickXORHashStream(tmp))
	assert.Equal(t, SHA1Hash(&content), SHA1HashStream(tmp))
	assert.Equal(t, SHA256Hash(&content), SHA256HashStream(tmp))
}
