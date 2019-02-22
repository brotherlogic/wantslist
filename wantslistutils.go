package main

import (
	"fmt"
	"sort"
	"time"

	"golang.org/x/net/context"

	pbrc "github.com/brotherlogic/recordcollection/proto"
	pb "github.com/brotherlogic/wantslist/proto"
)

func (s *Server) prodProcess(ctx context.Context) {
	s.processWantLists(ctx, s.listWait)
}

func (s *Server) processWantLists(ctx context.Context, d time.Duration) {
	for i, list := range s.config.Lists {
		if time.Now().After(time.Unix(list.LastProcessTime, 0).Add(d)) {
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
						r, err := s.rcBridge.getRecord(ctx, v.Want)
						if err == nil && r.GetMetadata().Category != pbrc.ReleaseMetadata_UNLISTENED && r.GetMetadata().Category != pbrc.ReleaseMetadata_STAGED {
							v.Status = pb.WantListEntry_COMPLETE
						}
					}
				}
			}

			list.LastProcessTime = time.Now().Unix()
			break
		}
	}

	s.save(ctx)
}
