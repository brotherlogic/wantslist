package main

import (
	"fmt"
	"time"

	"golang.org/x/net/context"

	rcpb "github.com/brotherlogic/recordcollection/proto"
	pb "github.com/brotherlogic/wantslist/proto"
)

//AddWantList adds a want list
func (s *Server) AddWantList(ctx context.Context, req *pb.AddWantListRequest) (*pb.AddWantListResponse, error) {
	config, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	if len(config.Lists) >= 8 {
		return nil, fmt.Errorf("You can't have more than 6 lists - you have %v", len(config.Lists))
	}

	req.Add.Year = int32(time.Now().Year())
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

//ClientUpdate on an updated record
func (s *Server) ClientUpdate(ctx context.Context, req *rcpb.ClientUpdateRequest) (*rcpb.ClientUpdateResponse, error) {
	return &rcpb.ClientUpdateResponse{}, s.prodProcess(ctx)
}
