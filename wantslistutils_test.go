package main

import (
	"testing"
)

func InitTestServer() *Server {
	s := Init()
	s.SkipLog = true
	return s
}

func TestFirst(t *testing.T) {
	blank()
}
