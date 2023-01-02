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

func TestAmendWantList(t *testing.T) {
	s := InitTestServer()

	_, err := s.AddWantList(context.Background(), &pb.AddWantListRequest{
		Add: &pb.WantList{
			Name:   "Testing",
			Budget: "test",
			Wants: []*pb.WantListEntry{
				{Want: 123},
				{Want: 456},
			},
		},
	})
	if err != nil {
		t.Fatalf("Bad wantlist add: %v", err)
	}

	_, err = s.AmendWantListItem(context.Background(), &pb.AmendWantListItemRequest{
		Name:  "Testing",
		OldId: 123,
		NewId: 124,
	})
	if err != nil {
		t.Fatalf("Bad wantlist amend: %v", err)
	}

	list, err := s.GetWantList(context.Background(), &pb.GetWantListRequest{Name: "Testing"})
	if err != nil {
		t.Fatalf("Bad wantlist retrieve: %v", err)
	}

	if len(list.GetLists()) != 1 || len(list.GetLists()[0].GetWants()) != 2 || list.GetLists()[0].GetWants()[0].Want != 124 {
		t.Errorf("List update failure: %v", list)
	}

	found := false
	for _, list := range list.GetLists() {
		for _, entry := range list.GetWants() {
			if entry.Want == 456 {
				found = true
			}
		}
	}

	if !found {
		t.Errorf("More amendment than expected: %v", list)
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

func TestDeleteWantsListFail(t *testing.T) {
	s := InitTestServer()

	s.GoServer.KSclient.Fail = true

	_, err := s.DeleteWantList(context.Background(), &pb.DeleteWantlistRequest{})

	if err == nil {
		t.Fatalf("Should have errored")
	}
}

func TestDeleteWantsList(t *testing.T) {
	s := InitTestServer()

	s.AddWantList(context.Background(), &pb.AddWantListRequest{Add: &pb.WantList{Name: "hello"}})
	s.AddWantList(context.Background(), &pb.AddWantListRequest{Add: &pb.WantList{Name: "goodbye"}})
	s.DeleteWantList(context.Background(), &pb.DeleteWantlistRequest{Name: "hello"})
	lists, err := s.GetWantList(context.Background(), &pb.GetWantListRequest{})

	if err != nil {
		t.Fatalf("Error in getting lists: %v -> %v", err, lists)
	}

	if len(lists.GetLists()) != 1 {
		t.Errorf("List was not removed")
	}

}
