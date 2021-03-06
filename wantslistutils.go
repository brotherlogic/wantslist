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
	config, err := s.load(ctx)
	if err == nil {
		err = s.processWantLists(ctx, config, s.listWait)
	}
	return err
}

func (s *Server) updateWant(ctx context.Context, v *pb.WantListEntry, list *pb.WantList) error {
	if v.Status == pb.WantListEntry_WANTED {
		r, err := s.rcBridge.getRecord(ctx, v.Want)
		s.Log(fmt.Sprintf("GOT Record: %v, %v", r, err))
		if err == nil && ((list.GetType() == pb.WantList_STANDARD && r.GetMetadata().Category != pbrc.ReleaseMetadata_UNLISTENED && r.GetMetadata().Category != pbrc.ReleaseMetadata_STAGED && r.GetMetadata().GetCategory() != pbrc.ReleaseMetadata_UNKNOWN) ||
			(list.GetType() == pb.WantList_STANDARD && r.GetMetadata().Category != pbrc.ReleaseMetadata_UNLISTENED)) {
			s.RaiseIssue("Wantlist Update", fmt.Sprintf("Transition to complete because category is %v", r.GetMetadata().Category))
			v.Status = pb.WantListEntry_COMPLETE
		} else if err != nil {
			s.Log(fmt.Sprintf("Error record: %v", err))
			want, err := s.wantBridge.get(ctx, v.Want)
			if err == nil && want.Level != pbrw.MasterWant_ANYTIME_LIST && want.Level != pbrw.MasterWant_STAGED_TO_BE_ADDED {
				return s.wantBridge.want(ctx, v.Want)
			}
		}
	}
	return nil
}

func (s *Server) processWantLists(ctx context.Context, config *pb.Config, d time.Duration) error {
	for _, list := range config.Lists {
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
					s.updateWant(ctx, v, list)
				}
			}

			list.LastProcessTime = time.Now().Unix()
			break
		}
	}

	config = s.cleanWantlists(ctx, config)
	return s.save(ctx, config)
}

func (s *Server) cleanWantlists(ctx context.Context, config *pb.Config) *pb.Config {
	i := 0
	for _, list := range config.Lists {
		count := 0
		for _, elem := range list.Wants {
			if elem.Status == pb.WantListEntry_COMPLETE {
				count++
			}
		}

		if len(list.Wants) != count {
			config.Lists[i] = list
			i++
		}
	}
	config.Lists = config.Lists[:i]

	newwantlists := []*pb.WantList{}
	for _, list := range config.Lists {
		if int32(time.Now().Year()) == list.GetYear() {
			newwantlists = append(newwantlists, list)
		}
	}

	config.Lists = newwantlists
	return config
}
