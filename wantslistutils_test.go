package main

import (
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/brotherlogic/keystore/client"
	pbrw "github.com/brotherlogic/recordwants/proto"
	"golang.org/x/net/context"

	pbgd "github.com/brotherlogic/godiscogs"
	pbrc "github.com/brotherlogic/recordcollection/proto"
	pb "github.com/brotherlogic/wantslist/proto"
)

type testRcBridge struct {
	returnComplete bool
	fail           bool
}

func (t *testRcBridge) getRecord(ctx context.Context, id int32) (*pbrc.Record, error) {
	if t.fail {
		return nil, fmt.Errorf("Built to fail")
	}
	if t.returnComplete {
		return &pbrc.Record{Release: &pbgd.Release{Id: id}, Metadata: &pbrc.ReleaseMetadata{Category: pbrc.ReleaseMetadata_HIGH_SCHOOL}}, nil
	}
	return &pbrc.Record{Release: &pbgd.Release{Id: id}, Metadata: &pbrc.ReleaseMetadata{Category: pbrc.ReleaseMetadata_STAGED}}, nil
}

type testWantBridge struct {
	fail bool
}

func (t *testWantBridge) want(ctx context.Context, id int32) error {
	if t.fail {
		return fmt.Errorf("Built to fail")
	}
	return nil
}

func (t *testWantBridge) get(ctx context.Context, id int32) (*pbrw.MasterWant, error) {
	return &pbrw.MasterWant{}, nil
}

func InitTestServer() *Server {
	s := Init()
	s.SkipLog = true
	s.SkipIssue = true
	s.GoServer.KSclient = *keystoreclient.GetTestClient(".test")
	s.wantBridge = &testWantBridge{}
	s.rcBridge = &testRcBridge{}

	d, err := time.ParseDuration("0s")
	if err != nil {
		log.Fatalf("Error parsing time")
	}
	s.listWait = d

	return s
}

func TestFirstEntrySet(t *testing.T) {
	s := InitTestServer()
	s.AddWantList(context.Background(), &pb.AddWantListRequest{
		Add: &pb.WantList{
			Name: "TestList",
			Year: int32(time.Now().Year()),
			Wants: []*pb.WantListEntry{
				&pb.WantListEntry{Index: 1, Want: 123},
				&pb.WantListEntry{Index: 2, Want: 125},
			},
		},
	})

	s.prodProcess(context.Background())

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

	s.prodProcess(context.Background())

	lists, err := s.GetWantList(context.Background(), &pb.GetWantListRequest{})
	if err != nil {
		t.Fatalf("Error getting wants: %v", err)
	}
	if lists.Lists[0].Wants[1].Status != pb.WantListEntry_WANTED {
		t.Errorf("Want has not been updated following first complete")
	}
}

func TestFirstEntryUpdatedToCollection(t *testing.T) {
	s := InitTestServer()
	s.AddWantList(context.Background(), &pb.AddWantListRequest{
		Add: &pb.WantList{
			Name: "TestList",
			Wants: []*pb.WantListEntry{
				&pb.WantListEntry{Index: 1, Want: 123, Status: pb.WantListEntry_WANTED},
				&pb.WantListEntry{Index: 2, Want: 125},
			},
		},
	})

	s.prodProcess(context.Background())

	lists, err := s.GetWantList(context.Background(), &pb.GetWantListRequest{})
	if err != nil {
		t.Fatalf("Error getting wants: %v", err)
	}
	if lists.Lists[0].Wants[0].Status != pb.WantListEntry_WANTED {
		t.Errorf("Want has not been updated following first complete: %v", lists.Lists[0].Wants[0])
	}
}

func TestFirstEntryUpdatedToComplete(t *testing.T) {
	s := InitTestServer()
	s.rcBridge = &testRcBridge{returnComplete: true}
	s.AddWantList(context.Background(), &pb.AddWantListRequest{
		Add: &pb.WantList{
			Name: "TestList",
			Wants: []*pb.WantListEntry{
				&pb.WantListEntry{Index: 1, Want: 123, Status: pb.WantListEntry_WANTED},
				&pb.WantListEntry{Index: 2, Want: 125},
			},
		},
	})

	s.prodProcess(context.Background())

	lists, err := s.GetWantList(context.Background(), &pb.GetWantListRequest{})
	if err != nil {
		t.Fatalf("Error getting wants: %v", err)
	}
	if lists.Lists[0].Wants[0].Status != pb.WantListEntry_COMPLETE {
		t.Errorf("Want has not been updated to complete: %v", lists.Lists[0].Wants[0])
	}
}

func TestUpdateWant(t *testing.T) {
	s := InitTestServer()
	s.rcBridge = &testRcBridge{fail: true}

	err := s.updateWant(context.Background(), &pb.WantListEntry{Status: pb.WantListEntry_WANTED})
	if err != nil {
		t.Errorf("Bad update did not fail")
	}
}
