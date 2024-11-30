package node

import (
	"testing"
)

func TestFallback(t *testing.T) {
	safrole(false)
}

func TestSafrole(t *testing.T) {
	safrole(true)
}

func TestFib(t *testing.T) {
	jamtest("fib")
}

func TestMegatron(t *testing.T) {
	jamtest("megatron")
}
