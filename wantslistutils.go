package main

import (
	"fmt"
	"sort"
	"time"

	"golang.org/x/net/context"

	pbrc "github.com/brotherlogic/recordcollection/proto"
	pbrw "github.com/brotherlogic/recordwants/proto"
	pb "github.com/brotherlogic/wantslist/proto"
)

func (s *Server) prodProcess(ctx context.Context) error {
	s.processWantLists(ctx, s.listWait)
	return nil
}

func (s *Server) updateWant(ctx context.Context, v *pb.WantListEntry) error {
	if v.Status == pb.WantListEntry_WANTED {
		r, err := s.rcBridge.getRecord(ctx, v.Want)
		s.Log(fmt.Sprintf("GOT Record: %v, %v", r, err))
		if err == nil && r.GetMetadata().Category != pbrc.ReleaseMetadata_UNLISTENED && r.GetMetadata().Category != pbrc.ReleaseMetadata_STAGED {
			s.RaiseIssue(ctx, "Wantlist Update", fmt.Sprintf("Transition to complete because category is %v", r.GetMetadata().Category), false)
			v.Status = pb.WantListEntry_COMPLETE
		} else if err != nil {
			s.Log(fmt.Sprintf("Error record: %v", err))
			want, err := s.wantBridge.get(ctx, v.Want)
			if err == nil && want.Level != pbrw.MasterWant_LIST && want.Level != pbrw.MasterWant_STAGED_TO_BE_ADDED {
				return s.wantBridge.want(ctx, v.Want)
			}
		}
	}
	return nil
}

func (s *Server) processWantLists(ctx context.Context, d time.Duration) {
	for _, list := range s.config.Lists {
		if time.Now().After(time.Unix(list.LastProcessTime, 0).Add(d)) {
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

	s.cleanWantlists(ctx)
	s.save(ctx)
}

func (s *Server) cleanWantlists(ctx context.Context) {
	i := 0
	for _, list := range s.config.Lists {
		count := 0
		for _, elem := range list.Wants {
			if elem.Status == pb.WantListEntry_COMPLETE {
				count++
			}
		}

		if len(list.Wants) != count {
			s.config.Lists[i] = list
			i++
		}
	}
	s.config.Lists = s.config.Lists[:i]

	newwantlists := []*pb.WantList{}
	for _, list := range s.config.Lists {
		if int32(time.Now().Year()) == list.GetYear() {
			newwantlists = append(newwantlists, list)
		}
	}

	s.config.Lists = newwantlists
}
