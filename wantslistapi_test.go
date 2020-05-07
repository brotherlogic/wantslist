package main

import (
	"context"
	"testing"

	pb "github.com/brotherlogic/wantslist/proto"
)

func TestWantsList(t *testing.T) {
	s := InitTestServer()

	s.AddWantList(context.Background(), &pb.AddWantListRequest{Add: &pb.WantList{Name: "hello"}})
	lists, err := s.GetWantList(context.Background(), &pb.GetWantListRequest{})

	if err != nil {
		t.Fatalf("Error in getting lists: %v", err)
	}

	if len(lists.Lists) != 1 {
		t.Errorf("Wrong number of lists: %v", len(s.config.Lists))
	}
}

func TestWantsListAddFail(t *testing.T) {
	s := InitTestServer()

	for i := 0; i < 9; i++ {
		s.AddWantList(context.Background(), &pb.AddWantListRequest{Add: &pb.WantList{Name: "hello"}})
	}

	out, err := s.AddWantList(context.Background(), &pb.AddWantListRequest{Add: &pb.WantList{Name: "hello"}})
	if err == nil {
		t.Errorf("Should have failed: %v", out)
	}
}
