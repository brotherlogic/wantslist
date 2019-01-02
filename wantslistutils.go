package main

import (
	"fmt"
	"sort"
	"time"

	"golang.org/x/net/context"

	pbt "github.com/brotherlogic/tracer/proto"
	pb "github.com/brotherlogic/wantslist/proto"
)

func (s *Server) processWantLists(ctx context.Context) {
	ctx = s.LogTrace(ctx, "processWantLists", time.Now(), pbt.Milestone_START_FUNCTION)
	for i, list := range s.config.Lists {
		s.LogTrace(ctx, fmt.Sprintf("Processing List %v", i), time.Now(), pbt.Milestone_MARKER)
		sort.SliceStable(list.Wants, func(i, j int) bool {
			return list.Wants[i].Index < list.Wants[j].Index
		})

		var toUpdateToWanted *pb.WantListEntry
		if list.Wants[0].Status == pb.WantListEntry_UNPROCESSED {
			toUpdateToWanted = list.Wants[0]
		} else {
			for i := range list.Wants[1:] {
				if list.Wants[i].Status == pb.WantListEntry_COMPLETE && list.Wants[i+1].Status == pb.WantListEntry_UNPROCESSED {
					toUpdateToWanted = list.Wants[i+1]
				}
			}
		}
		s.LogTrace(ctx, fmt.Sprintf("Identified update for list %v", i), time.Now(), pbt.Milestone_MARKER)

		if toUpdateToWanted != nil {
			err := s.wantBridge.want(ctx, toUpdateToWanted.Want)
			s.Log(fmt.Sprintf("Updating %v to WANTED with error %v", toUpdateToWanted.Want, err))
			if err == nil {
				toUpdateToWanted.Status = pb.WantListEntry_WANTED
			}
		}

		if toUpdateToWanted == nil {
			for _, v := range list.Wants {
				if v.Status == pb.WantListEntry_WANTED {
					_, err := s.rcBridge.getRecord(ctx, v.Want)
					if err == nil {
						v.Status = pb.WantListEntry_IN_COLLECTION
					}
				}
			}
		}
	}

	s.save(ctx)
	s.LogTrace(ctx, "processWantLists", time.Now(), pbt.Milestone_END_FUNCTION)
}
