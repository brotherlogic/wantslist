package main

import (
	"fmt"
	"time"

	pb "github.com/brotherlogic/wantslist/proto"
	"golang.org/x/net/context"
)

//AddWantList adds a want list
func (s *Server) AddWantList(ctx context.Context, req *pb.AddWantListRequest) (*pb.AddWantListResponse, error) {
	if len(s.config.Lists) >= 6 {
		return nil, fmt.Errorf("You can't have more than 6 lists - you have %v", len(s.config.Lists))
	}

	req.Add.Year = int32(time.Now().Year())
	s.config.Lists = append(s.config.Lists, req.Add)
	s.save(ctx)
	return &pb.AddWantListResponse{}, nil
}

//GetWantList gets a want list
func (s *Server) GetWantList(ctx context.Context, req *pb.GetWantListRequest) (*pb.GetWantListResponse, error) {
	return &pb.GetWantListResponse{Lists: s.config.Lists}, nil
}
