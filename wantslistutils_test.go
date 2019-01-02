package main

import (
	"fmt"
	"testing"

	"github.com/brotherlogic/keystore/client"
	pb "github.com/brotherlogic/wantslist/proto"
	"golang.org/x/net/context"
)

type testWantBridge struct {
	fail bool
}

func (t *testWantBridge) want(ctx context.Context, id int32) error {
	if t.fail {
		return fmt.Errorf("Built to fail")
	}
	return nil
}

func InitTestServer() *Server {
	s := Init()
	s.SkipLog = true
	s.GoServer.KSclient = *keystoreclient.GetTestClient(".test")
	s.wantBridge = &testWantBridge{}
	return s
}

func TestFirstEntrySet(t *testing.T) {
	s := InitTestServer()
	s.AddWantList(context.Background(), &pb.AddWantListRequest{
		Add: &pb.WantList{
			Name: "TestList",
			Wants: []*pb.WantListEntry{
				&pb.WantListEntry{Index: 1, Want: 123},
				&pb.WantListEntry{Index: 2, Want: 125},
			},
		},
	})

	s.processWantLists(context.Background())

	lists, err := s.GetWantList(context.Background(), &pb.GetWantListRequest{})
	if err != nil {
		t.Fatalf("Error getting wants: %v", err)
	}
	if lists.Lists[0].Wants[0].Status != pb.WantListEntry_WANTED {
		t.Errorf("Want has not been updated")
	}
}

func TestFirstEntryUpdated(t *testing.T) {
	s := InitTestServer()
	s.AddWantList(context.Background(), &pb.AddWantListRequest{
		Add: &pb.WantList{
			Name: "TestList",
			Wants: []*pb.WantListEntry{
				&pb.WantListEntry{Index: 1, Want: 123, Status: pb.WantListEntry_COMPLETE},
				&pb.WantListEntry{Index: 2, Want: 125},
			},
		},
	})

	s.processWantLists(context.Background())

	lists, err := s.GetWantList(context.Background(), &pb.GetWantListRequest{})
	if err != nil {
		t.Fatalf("Error getting wants: %v", err)
	}
	if lists.Lists[0].Wants[1].Status != pb.WantListEntry_WANTED {
		t.Errorf("Want has not been updated following first complete")
	}
}
