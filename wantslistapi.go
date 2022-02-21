package main

import (
	"fmt"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	rcpb "github.com/brotherlogic/recordcollection/proto"
	pb "github.com/brotherlogic/wantslist/proto"
)

func (s *Server) AddWantListItem(ctx context.Context, req *pb.AddWantListItemRequest) (*pb.AddWantListItemResponse, error) {
	config, err := s.load(ctx)
	if err != nil {
		return nil, err
	}

	for _, list := range config.GetLists() {
		if list.GetName() == req.GetListName() {
			for _, elem := range list.GetWants() {
				if elem.GetWant() == req.GetEntry().GetWant() {
					return &pb.AddWantListItemResponse{}, nil
				}
			}

			list.Wants = append(list.Wants, req.GetEntry())
			return &pb.AddWantListItemResponse{}, s.save(ctx, config)
		}
	}

	return nil, fmt.Errorf("Cannot find list: %v", req.GetListName())
}

func (s *Server) DeleteWantListItem(ctx context.Context, req *pb.DeleteWantListItemRequest) (*pb.DeleteWantListItemResponse, error) {
	config, err := s.load(ctx)
	if err != nil {
		return nil, err
	}

	for _, list := range config.GetLists() {
		if list.GetName() == req.GetListName() {
			var wants []*pb.WantListEntry
			for _, elem := range list.GetWants() {
				if elem.GetWant() != req.GetEntry().GetWant() {
					wants = append(wants, elem)
				} else {
					s.wantBridge.unwant(ctx, elem.GetWant(), list.GetBudget())
				}
			}
			list.Wants = wants
		}
	}

	return &pb.DeleteWantListItemResponse{}, s.save(ctx, config)
}

//AddWantList adds a want list
func (s *Server) AddWantList(ctx context.Context, req *pb.AddWantListRequest) (*pb.AddWantListResponse, error) {
	config, err := s.load(ctx)
	if err != nil {
		return nil, err
	}

	if len(config.Lists) > 3 {
		return nil, fmt.Errorf("You need to have 3 lists - you have %v", len(config.Lists))
	}

	req.Add.Year = int32(time.Now().Year())
	req.Add.TimeAdded = time.Now().Unix()
	config.Lists = append(config.Lists, req.Add)
	config.LastChange = time.Now().Unix()

	return &pb.AddWantListResponse{}, s.save(ctx, config)
}

//GetWantList gets a want list
func (s *Server) GetWantList(ctx context.Context, req *pb.GetWantListRequest) (*pb.GetWantListResponse, error) {
	config, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	return &pb.GetWantListResponse{Lists: config.Lists}, nil
}

//GetWantList gets a want list
func (s *Server) DeleteWantList(ctx context.Context, req *pb.DeleteWantlistRequest) (*pb.DeleteWantlistResponse, error) {
	config, err := s.load(ctx)
	if err != nil {
		return nil, err
	}

	var lists []*pb.WantList
	for _, list := range config.GetLists() {
		if list.GetName() != req.GetName() {
			lists = append(lists, list)
		}
	}
	config.Lists = lists

	return &pb.DeleteWantlistResponse{}, s.save(ctx, config)
}

//ClientUpdate on an updated record
func (s *Server) ClientUpdate(ctx context.Context, req *rcpb.ClientUpdateRequest) (*rcpb.ClientUpdateResponse, error) {
	config, err := s.load(ctx)
	if err != nil {
		return nil, err
	}

	r, err := s.rcBridge.getSpRecord(ctx, req.GetInstanceId())
	if err != nil {
		// Don't process a deleted record
		if status.Convert(err).Code() == codes.OutOfRange {
			return &rcpb.ClientUpdateResponse{}, nil
		}
		return nil, err
	}

	for _, list := range config.GetLists() {
		for _, want := range list.GetWants() {
			if want.Want == r.GetRelease().GetId() {
				if want.GetStatus() == pb.WantListEntry_WANTED {
					s.CtxLog(ctx, fmt.Sprintf("Marking %v from %v as LIMBO", want.Want, list.GetName()))
					want.Status = pb.WantListEntry_LIMBO
					return &rcpb.ClientUpdateResponse{}, s.prodProcess(ctx, config)
				} else if want.GetStatus() == pb.WantListEntry_LIMBO {
					if list.GetType() == pb.WantList_ALL_IN && r.GetMetadata().GetCategory() == rcpb.ReleaseMetadata_STAGED {
						want.Status = pb.WantListEntry_COMPLETE
						return &rcpb.ClientUpdateResponse{}, s.prodProcess(ctx, config)
					}
				}
			}
		}
	}

	return &rcpb.ClientUpdateResponse{}, s.prodProcess(ctx, config)
}
