package main

import "golang.org/x/net/context"
import pb "github.com/brotherlogic/wantslist/proto"

//AddWantList adds a want list
func (s *Server) AddWantList(ctx context.Context, req *pb.AddWantListRequest) (*pb.AddWantListResponse, error) {
	s.config.Lists = append(s.config.Lists, req.Add)
	return &pb.AddWantListResponse{}, nil
}
