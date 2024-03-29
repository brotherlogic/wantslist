package main

import (
	"fmt"
	"testing"
	"time"

	"golang.org/x/net/context"

	pbgd "github.com/brotherlogic/godiscogs/proto"
	keystoreclient "github.com/brotherlogic/keystore/client"
	rbc "github.com/brotherlogic/recordbudget/client"
	rbpb "github.com/brotherlogic/recordbudget/proto"
	rcc "github.com/brotherlogic/recordcollection/client"
	pbrc "github.com/brotherlogic/recordcollection/proto"
	pbrw "github.com/brotherlogic/recordwants/proto"
	pb "github.com/brotherlogic/wantslist/proto"
)

type testWantBridge struct {
	fail bool
}

func (t *testWantBridge) want(ctx context.Context, id int32, retire int64, budget string) error {
	if t.fail {
		return fmt.Errorf("Built to fail")
	}
	return nil
}

func (t *testWantBridge) unwant(ctx context.Context, id int32, budget, reason string) error {
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
	s.GoServer.KSclient.Save(context.Background(), KEY, &pb.Config{})
	s.wantBridge = &testWantBridge{}
	s.rcclient = &rcc.RecordCollectionClient{Test: true}
	s.budgetClient = &rbc.RecordBudgetClient{Test: true}

	s.budgetClient.AddBudget(&rbpb.Budget{Name: "basic", Remaining: 10})

	return s
}

func TestAddFail(t *testing.T) {
	s := InitTestServer()
	s.GoServer.KSclient.Fail = true
	_, err := s.AddWantList(context.Background(), &pb.AddWantListRequest{
		Add: &pb.WantList{
			Name:   "TestList",
			Budget: "basic",
			Year:   int32(time.Now().Year()),
			Wants: []*pb.WantListEntry{
				&pb.WantListEntry{Index: 1, Want: 123},
				&pb.WantListEntry{Index: 2, Want: 125},
			},
		},
	})

	if err == nil {
		t.Errorf("Should have failed")
	}
}

func TestGetCosts(t *testing.T) {
	s := InitTestServer()
	_, err := s.AddWantList(context.Background(), &pb.AddWantListRequest{
		Add: &pb.WantList{
			Name:   "TestList",
			Budget: "basic",
			Year:   int32(time.Now().Year()),
			Wants: []*pb.WantListEntry{
				&pb.WantListEntry{Index: 1, Want: 123},
				&pb.WantListEntry{Index: 2, Want: 125},
			},
		},
	})

	if err != nil {
		t.Errorf("Error on add list: %v", err)
	}

	s.ClientUpdate(context.Background(), &pbrc.ClientUpdateRequest{})

	list, err := s.GetWantList(context.Background(), &pb.GetWantListRequest{Name: "TestList"})
	if err != nil {
		t.Fatalf("Error on get list: %v", err)
	}

	if list.GetLists()[0].GetWants()[0].GetEstimatedCost() == 0 {
		t.Errorf("Cost was not recovered: %v", list)
	}
}

func TestFirstEntrySet(t *testing.T) {
	s := InitTestServer()
	s.AddWantList(context.Background(), &pb.AddWantListRequest{
		Add: &pb.WantList{
			Name:   "TestList",
			Budget: "basic",
			Year:   int32(time.Now().Year()),
			Wants: []*pb.WantListEntry{
				&pb.WantListEntry{Index: 1, Want: 123},
				&pb.WantListEntry{Index: 2, Want: 125},
			},
		},
	})

	s.ClientUpdate(context.Background(), &pbrc.ClientUpdateRequest{})

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
	_, err := s.AddWantList(context.Background(), &pb.AddWantListRequest{
		Add: &pb.WantList{
			Name:   "TestList",
			Budget: "basic",
			Wants: []*pb.WantListEntry{
				&pb.WantListEntry{Index: 1, Want: 123, Status: pb.WantListEntry_COMPLETE},
				&pb.WantListEntry{Index: 2, Want: 125},
			},
		},
	})
	if err != nil {
		t.Fatalf("Bad add list: %v", err)
	}

	// Blank update does a prod procss
	s.ClientUpdate(context.Background(), &pbrc.ClientUpdateRequest{})

	lists, err := s.GetWantList(context.Background(), &pb.GetWantListRequest{})
	if err != nil {
		t.Fatalf("Error getting wants: %v", err)
	}

	if len(lists.GetLists()) == 0 || len(lists.GetLists()[0].GetWants()) < 2 {
		t.Fatalf("Bad list return: %v", lists)
	}

	if lists.Lists[0].Wants[1].Status != pb.WantListEntry_WANTED {
		t.Errorf("Want has not been updated following first complete")
	}
}

func TestFirstEntryUpdatedToCollection(t *testing.T) {
	s := InitTestServer()
	s.AddWantList(context.Background(), &pb.AddWantListRequest{
		Add: &pb.WantList{
			Name:   "TestList",
			Budget: "basic",
			Wants: []*pb.WantListEntry{
				&pb.WantListEntry{Index: 1, Want: 123, Status: pb.WantListEntry_WANTED},
				&pb.WantListEntry{Index: 2, Want: 125},
			},
		},
	})

	// Blank update does a prod procss
	s.ClientUpdate(context.Background(), &pbrc.ClientUpdateRequest{})

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
	s.rcclient.AddRecord(&pbrc.Record{Release: &pbgd.Release{Id: 123, InstanceId: 1234}, Metadata: &pbrc.ReleaseMetadata{Category: pbrc.ReleaseMetadata_HIGH_SCHOOL}})
	s.AddWantList(context.Background(), &pb.AddWantListRequest{
		Add: &pb.WantList{
			Name:   "TestList1",
			Budget: "basic",
			Wants: []*pb.WantListEntry{
				&pb.WantListEntry{Index: 1, Want: 123, Status: pb.WantListEntry_WANTED},
				&pb.WantListEntry{Index: 2, Want: 125},
			},
		},
	})

	// Blank update does a prod procss
	s.ClientUpdate(context.Background(), &pbrc.ClientUpdateRequest{InstanceId: 1234})
	s.ClientUpdate(context.Background(), &pbrc.ClientUpdateRequest{InstanceId: 1234})

	lists, err := s.GetWantList(context.Background(), &pb.GetWantListRequest{})
	if err != nil {
		t.Fatalf("error getting wants: %v", err)
	}
	if lists.Lists[0].Wants[0].Status != pb.WantListEntry_COMPLETE {
		t.Errorf("want has not been updated to complete: %v", lists.Lists[0].Wants[0])
	}
}

func TestBudgetUpdate(t *testing.T) {
	s := InitTestServer()
	s.AddWantList(context.Background(), &pb.AddWantListRequest{
		Add: &pb.WantList{
			Name:   "TestListBudget",
			Budget: "basic",
			Type:   pb.WantList_ALL_IN,
			Wants: []*pb.WantListEntry{
				&pb.WantListEntry{Index: 1, Want: 123, Status: pb.WantListEntry_WANTED},
				&pb.WantListEntry{Index: 2, Want: 125},
			},
		},
	})

	s.lastRun = time.Now().Add(-time.Hour * 24)
	s.ClientUpdate(context.Background(), &pbrc.ClientUpdateRequest{})

	wl, err := s.GetWantList(context.Background(), &pb.GetWantListRequest{Name: "TestListBudget"})
	if err != nil {
		t.Errorf("Bad get: %v", err)
	}

	for _, list := range wl.GetLists() {
		for _, entry := range list.GetWants() {
			if entry.GetStatus() != pb.WantListEntry_WANTED {
				t.Fatalf("Should be unprocessed: %v", entry)
			}
		}
	}

	s.budgetClient.AddBudget(&rbpb.Budget{Name: "basic", Remaining: 0})
	s.lastRun = time.Now().Add(-time.Hour * 24)
	s.ClientUpdate(context.Background(), &pbrc.ClientUpdateRequest{})

	wl, err = s.GetWantList(context.Background(), &pb.GetWantListRequest{Name: "TestListBudget"})
	if err != nil {
		t.Errorf("Bad get: %v", err)
	}

	for _, list := range wl.GetLists() {
		for _, entry := range list.GetWants() {
			if entry.GetStatus() != pb.WantListEntry_UNPROCESSED {
				t.Errorf("Should be unprocessed: %v", entry)
			}
		}
	}
}

func TestBoughtRecordIsMarkedComplete(t *testing.T) {
	s := InitTestServer()
	_, err := s.AddWantList(context.Background(), &pb.AddWantListRequest{
		Add: &pb.WantList{
			Name:   "TestListBudget",
			Budget: "basic",
			Type:   pb.WantList_YEARLY,
			Wants: []*pb.WantListEntry{
				&pb.WantListEntry{Index: 1, Want: 1234, Status: pb.WantListEntry_WANTED},
				&pb.WantListEntry{Index: 2, Want: 12345, Status: pb.WantListEntry_COMPLETE},
			},
		},
	})

	if err != nil {
		t.Fatalf("Bad add of wantlist: %v", err)
	}

	s.rcclient.AddRecord(&pbrc.Record{Release: &pbgd.Release{Id: 1234, InstanceId: 12}})
	s.rcclient.AddRecord(&pbrc.Record{Release: &pbgd.Release{Id: 12345555, InstanceId: 1}})
	s.ClientUpdate(context.Background(), &pbrc.ClientUpdateRequest{InstanceId: 1})

	mapper := make(map[int]pb.WantListEntry_Status)
	wl, err := s.GetWantList(context.Background(), &pb.GetWantListRequest{Name: "TestListBudget"})
	if err != nil {
		t.Fatalf("Bad list get: %v", err)
	}
	for _, list := range wl.GetLists() {
		for _, entry := range list.GetWants() {
			mapper[int(entry.GetWant())] = entry.GetStatus()
		}
	}

	if mapper[1234] == pb.WantListEntry_WANTED {
		t.Errorf("Entry was not moved from complete: %v", wl)
	}
	if mapper[12345] != pb.WantListEntry_UNPROCESSED {
		t.Errorf("Entry was not marked wanted: %v", wl)
	}
}

func TestAddClearsWant(t *testing.T) {
	s := InitTestServer()
	_, err := s.AddWantList(context.Background(), &pb.AddWantListRequest{
		Add: &pb.WantList{
			Name:   "TestListBudget",
			Budget: "basic",
			Type:   pb.WantList_ALL_IN,
			Wants: []*pb.WantListEntry{
				&pb.WantListEntry{Index: 1, Want: 1234, Status: pb.WantListEntry_WANTED},
				&pb.WantListEntry{Index: 2, Want: 12345, Status: pb.WantListEntry_WANTED},
			},
		},
	})
	if err != nil {
		t.Fatalf("Bad add: %v", err)
	}

	// This triggers the update and nulls out the time.
	s.ClientUpdate(context.Background(), &pbrc.ClientUpdateRequest{InstanceId: 12})

	mapper := make(map[int]pb.WantListEntry_Status)
	wl, err := s.GetWantList(context.Background(), &pb.GetWantListRequest{Name: "TestListBudget"})
	if err != nil {
		t.Fatalf("Bad list get: %v", err)
	}
	for _, list := range wl.GetLists() {
		for _, entry := range list.GetWants() {
			mapper[int(entry.GetWant())] = entry.GetStatus()
		}
	}

	if mapper[1234] != pb.WantListEntry_WANTED {
		t.Errorf("Entry is still not wanted: %v", wl)
	}
	if mapper[12345] != pb.WantListEntry_WANTED {
		t.Errorf("Entry is still not wanted: %v", wl)
	}

	// This update should trigger the update logic
	s.rcclient.AddRecord(&pbrc.Record{Release: &pbgd.Release{Id: 1234, InstanceId: 12}})
	s.rcclient.AddRecord(&pbrc.Record{Release: &pbgd.Release{Id: 12345555, InstanceId: 1}})
	s.ClientUpdate(context.Background(), &pbrc.ClientUpdateRequest{InstanceId: 12})

	mapper = make(map[int]pb.WantListEntry_Status)
	wl, err = s.GetWantList(context.Background(), &pb.GetWantListRequest{Name: "TestListBudget"})
	if err != nil {
		t.Fatalf("Bad list get: %v", err)
	}
	for _, list := range wl.GetLists() {
		for _, entry := range list.GetWants() {
			mapper[int(entry.GetWant())] = entry.GetStatus()
		}
	}

	if mapper[1234] == pb.WantListEntry_WANTED {
		t.Errorf("Entry is still  wanted: %v", wl)
	}
	if mapper[12345] != pb.WantListEntry_UNPROCESSED {
		t.Errorf("Entry is still  wanted: %v", wl)
	}
}
