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
		t.Fatalf("Error in getting lists: %v -> %v", err, lists)
	}

}

func TestGetWantsListFail(t *testing.T) {
	s := InitTestServer()

	s.AddWantList(context.Background(), &pb.AddWantListRequest{Add: &pb.WantList{Name: "hello"}})

	s.GoServer.KSclient.Fail = true

	_, err := s.GetWantList(context.Background(), &pb.GetWantListRequest{})

	if err == nil {
		t.Fatalf("Should have errored")
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
