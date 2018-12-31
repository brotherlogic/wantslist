package main

import (
	"context"
	"testing"

	pb "github.com/brotherlogic/wantslist/proto"
)

func TestAddWantsList(t *testing.T) {
	s := InitTestServer()

	s.AddWantList(context.Background(), &pb.AddWantListRequest{Add: &pb.WantList{Name: "hello"}})

	if len(s.config.Lists) != 1 {
		t.Errorf("Wrong number of lists: %v", len(s.config.Lists))
	}
}
