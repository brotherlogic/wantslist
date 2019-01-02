package main

import (
	"testing"

	"github.com/brotherlogic/keystore/client"
)

func InitTestServer() *Server {
	s := Init()
	s.SkipLog = true
	s.GoServer.KSclient = *keystoreclient.GetTestClient(".test")
	return s
}

func TestFirst(t *testing.T) {
	blank()
}
