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

func (s *Server) AmendWantListItem(ctx context.Context, req *pb.AmendWantListItemRequest) (*pb.AmendWantListItemResponse, error) {
	config, err := s.load(ctx)
	if err != nil {
		return nil, err
	}

	for _, list := range config.GetLists() {
		if list.GetName() == req.GetName() {
			changed := false
			for _, entry := range list.GetWants() {
				// Unwant the old
				if entry.GetWant() == req.GetOldId() {
					if entry.GetStatus() == pb.WantListEntry_WANTED {
						err := s.wantBridge.unwant(ctx, entry.GetWant(), list.GetBudget(), "Unwanting for Amend")
						if err != nil {
							return nil, err
						}
					}

					// Reset
					entry.Want = req.NewId
					entry.Status = pb.WantListEntry_UNPROCESSED
					changed = true
				}
			}

			if !changed {
				return nil, status.Errorf(codes.NotFound, "%v was not found in %v", req.OldId, req.Name)
			}

			return &pb.AmendWantListItemResponse{}, s.save(ctx, config)
		}
	}

	return nil, status.Errorf(codes.NotFound, fmt.Sprintf("%v was not found", req.GetName()))
}

func (s *Server) ForceUpdate(ctx context.Context, req *pb.ForceUpdateRequest) (*pb.ForceUpdateResponse, error) {
	config, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	rerr := s.prodProcess(ctx, config, true)
	if rerr != nil {
		return nil, rerr
	}

	return &pb.ForceUpdateResponse{}, s.save(ctx, config)
}

func (s *Server) DeleteWantListItem(ctx context.Context, req *pb.DeleteWantListItemRequest) (*pb.DeleteWantListItemResponse, error) {
	config, err := s.load(ctx)
	if err != nil {
		return nil, err
	}

	for _, list := range config.GetLists() {
		if list.GetName() == req.GetListName() {
			s.CtxLog(ctx, fmt.Sprintf("Found list called %v", req.GetListName()))
			var wants []*pb.WantListEntry
			found := false
			for _, elem := range list.GetWants() {
				if elem.GetWant() != req.GetEntry().GetWant() {
					wants = append(wants, elem)
				} else {
					found = true
					s.CtxLog(ctx, fmt.Sprintf("Found Want"))
					s.wantBridge.unwant(ctx, elem.GetWant(), list.GetBudget(), "Unwating in prep for delete item")
				}
			}

			s.CtxLog(ctx, fmt.Sprintf("Found %v", found))
			list.Wants = wants
		}
	}

	return &pb.DeleteWantListItemResponse{}, s.save(ctx, config)
}

// AddWantList adds a want list
func (s *Server) AddWantList(ctx context.Context, req *pb.AddWantListRequest) (*pb.AddWantListResponse, error) {
	config, err := s.load(ctx)
	if err != nil {
		return nil, err
	}

	for _, list := range config.GetLists() {
		if list.GetName() == req.Add.GetName() {
			return nil, status.Errorf(codes.AlreadyExists, "%v already exists", req.Add.GetName())
		}
	}

	req.Add.Year = int32(time.Now().Year())
	req.Add.TimeAdded = time.Now().Unix()
	config.Lists = append(config.Lists, req.Add)
	config.LastChange = time.Now().Unix()

	return &pb.AddWantListResponse{}, s.save(ctx, config)
}

// GetWantList gets a want list
func (s *Server) GetWantList(ctx context.Context, req *pb.GetWantListRequest) (*pb.GetWantListResponse, error) {
	config, err := s.load(ctx)
	if err != nil {
		return nil, err
	}

	var lists []*pb.WantList
	for _, l := range config.Lists {
		if req.GetName() == "" || req.GetName() == l.GetName() {
			lists = append(lists, l)
		}
	}

	return &pb.GetWantListResponse{Lists: lists}, nil
}

// GetWantList gets a want list
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

// ClientUpdate on an updated record
func (s *Server) ClientUpdate(ctx context.Context, req *rcpb.ClientUpdateRequest) (*rcpb.ClientUpdateResponse, error) {
	config, err := s.load(ctx)
	if err != nil {
		return nil, err
	}

	res, err := s.rcclient.GetRecord(ctx, &rcpb.GetRecordRequest{InstanceId: req.GetInstanceId()})
	if err != nil {
		s.CtxLog(ctx, fmt.Sprintf("Unable to locate %v -> %v", req.GetInstanceId(), err))
		// Don't process a deleted record
		if status.Convert(err).Code() == codes.OutOfRange {
			return &rcpb.ClientUpdateResponse{}, nil
		}
		return nil, err
	}
	r := res.GetRecord()

	found := false
	for _, list := range config.GetLists() {
		for _, want := range list.GetWants() {
			if want.Want == r.GetRelease().GetId() {
				found = true
				if want.GetStatus() == pb.WantListEntry_WANTED {
					s.CtxLog(ctx, fmt.Sprintf("Marking %v from %v as LIMBO (%v)", want.Want, list.GetName(), r))
					want.Status = pb.WantListEntry_LIMBO
					return &rcpb.ClientUpdateResponse{}, s.prodProcess(ctx, config, false)
				} else if want.GetStatus() == pb.WantListEntry_LIMBO {
					if (list.GetType() == pb.WantList_ALL_IN || list.GetType() == pb.WantList_RAPID || list.GetType() == pb.WantList_YEARLY) &&
						r.GetMetadata().GetCategory() == rcpb.ReleaseMetadata_STAGED ||
						r.GetMetadata().GetCategory() == rcpb.ReleaseMetadata_HIGH_SCHOOL ||
						r.GetMetadata().GetCategory() == rcpb.ReleaseMetadata_PRE_HIGH_SCHOOL ||
						r.GetMetadata().GetCategory() == rcpb.ReleaseMetadata_IN_COLLECTION {
						want.Status = pb.WantListEntry_COMPLETE
						return &rcpb.ClientUpdateResponse{}, s.prodProcess(ctx, config, false)
					}
				}
				s.CtxLog(ctx, fmt.Sprintf("Huh: %v, %v", want, r))
			}
		}
	}

	if !found {
		s.CtxLog(ctx, fmt.Sprintf("Unable to locate %v in any want lists", req.GetInstanceId()))
	}

	return &rcpb.ClientUpdateResponse{}, s.prodProcess(ctx, config, false)
}
