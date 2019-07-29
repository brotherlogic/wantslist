package main

import (
	"fmt"
	"sort"
	"time"

	"golang.org/x/net/context"

	pbrc "github.com/brotherlogic/recordcollection/proto"
	pb "github.com/brotherlogic/wantslist/proto"
)

func (s *Server) prodProcess(ctx context.Context) error {
	s.processWantLists(ctx, s.listWait)
	return nil
}

func (s *Server) updateWant(ctx context.Context, v *pb.WantListEntry) error {
	if v.Status == pb.WantListEntry_WANTED {
		r, err := s.rcBridge.getRecord(ctx, v.Want)
		if err == nil && r.GetMetadata().Category != pbrc.ReleaseMetadata_UNLISTENED && r.GetMetadata().Category != pbrc.ReleaseMetadata_STAGED {
			v.Status = pb.WantListEntry_COMPLETE
		} else if err != nil {
			s.Log(fmt.Sprintf("Error record: %v", err))
			return s.wantBridge.want(ctx, v.Want)
		}

		return s.wantBridge.want(ctx, v.Want)
	}
	return nil
}

func (s *Server) processWantLists(ctx context.Context, d time.Duration) {
	for _, list := range s.config.Lists {
		if time.Now().After(time.Unix(list.LastProcessTime, 0).Add(d)) {
			s.Log(fmt.Sprintf("Processing %v", list))
			sort.SliceStable(list.Wants, func(i2, j2 int) bool {
				return list.Wants[i2].Index < list.Wants[j2].Index
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
					s.updateWant(ctx, v)
				}
			}

			list.LastProcessTime = time.Now().Unix()
			break
		}
	}

	s.save(ctx)
}
