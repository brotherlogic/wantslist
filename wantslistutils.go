package main

import (
	"fmt"
	"math"
	"sort"
	"time"

	"golang.org/x/net/context"

	pbrc "github.com/brotherlogic/recordcollection/proto"
	pbrw "github.com/brotherlogic/recordwants/proto"
	pb "github.com/brotherlogic/wantslist/proto"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	togoMetric = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "wantslist_togo",
		Help: "The number of outstanding wants",
	}, []string{"list", "budget"})
	oldest = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "wantslist_oldest_cost",
		Help: "The number of outstanding wants",
	})
	newest = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "wantslist_newest_cost",
		Help: "The number of outstanding wants",
	})
)

func recordMetrics(config *pb.Config) {
	for _, list := range config.GetLists() {
		togo := 0
		for _, entry := range list.GetWants() {
			if entry.Status == pb.WantListEntry_WANTED {
				togo++
			}
		}
		togoMetric.With(prometheus.Labels{"list": list.GetName(), "budget": list.GetBudget()}).Set(float64(togo))
	}
}

func (s *Server) prodProcess(ctx context.Context, config *pb.Config) error {
	s.CtxLog(ctx, fmt.Sprintf("Running pproc: %v", time.Since(s.lastRun)))
	if time.Since(s.lastRun) < time.Hour {
		for _, list := range config.GetLists() {
			s.updateCosts(ctx, list)
		}
		return s.save(ctx, config)
	}
	s.lastRun = time.Now()
	return s.processWantLists(ctx, config)
}

func (s *Server) getRecord(ctx context.Context, id int32) (*pbrc.Record, error) {
	ids, err := s.rcclient.QueryRecords(ctx, &pbrc.QueryRecordsRequest{Query: &pbrc.QueryRecordsRequest_ReleaseId{int32(id)}})
	if err != nil {
		return nil, err
	}

	if len(ids.GetInstanceIds()) > 0 {
		rec, err := s.rcclient.GetRecord(ctx, &pbrc.GetRecordRequest{InstanceId: ids.GetInstanceIds()[0]})
		if err != nil {
			return nil, err
		}
		return rec.GetRecord(), err
	}

	return nil, fmt.Errorf("Cannot locate %v", id)
}

func (s *Server) updateWant(ctx context.Context, v *pb.WantListEntry, list *pb.WantList) error {
	if v.Status == pb.WantListEntry_WANTED {
		r, err := s.getRecord(ctx, v.Want)
		s.CtxLog(ctx, fmt.Sprintf("GOT Record: %v, %v", r, err))
		if err == nil && ((list.GetType() == pb.WantList_STANDARD &&
			r.GetMetadata().Category != pbrc.ReleaseMetadata_UNLISTENED &&
			r.GetMetadata().Category != pbrc.ReleaseMetadata_STAGED &&
			r.GetMetadata().GetCategory() != pbrc.ReleaseMetadata_UNKNOWN) ||
			(list.GetType() == pb.WantList_RAPID &&
				r.GetMetadata().Category != pbrc.ReleaseMetadata_UNLISTENED)) {
			v.Status = pb.WantListEntry_COMPLETE
		} else if err != nil {
			want, err := s.wantBridge.get(ctx, v.Want)
			s.CtxLog(ctx, fmt.Sprintf("Got want: %v, %v", want, err))
			if err != nil || want.Level != pbrw.MasterWant_ANYTIME_LIST || want.GetRetireTime() != list.GetRetireTime() {
				return s.wantBridge.want(ctx, v.Want, list.GetRetireTime(), list.GetBudget())
			}
		}
	} else if v.Status == pb.WantListEntry_UNPROCESSED {
		s.wantBridge.unwant(ctx, v.Want, list.GetBudget())
	}
	return nil
}

func (s *Server) updateCosts(ctx context.Context, list *pb.WantList) error {
	defer func() {
		costs := int32(0)
		for _, entry := range list.GetWants() {
			costs += entry.GetEstimatedCost()
		}
		list.OverallEstimatedCost = costs
	}()

	old := int64(math.MaxInt64)
	new := int64(0)
	for _, entry := range list.GetWants() {
		if entry.GetLastCostTime() < old {
			old = (entry.GetLastCostTime())
		}
		if entry.GetLastCostTime() > new {
			new = entry.GetLastCostTime()
		}
	}
	oldest.Set(float64(old))
	newest.Set(float64(new))

	for _, entry := range list.GetWants() {
		if time.Since(time.Unix(entry.GetLastCostTime(), 0)) > time.Hour*24*7 {
			price, err := s.rcclient.GetPrice(ctx, &pbrc.GetPriceRequest{Id: entry.GetWant()})
			if err != nil {
				return err
			}
			entry.EstimatedCost = int32(price.GetPrice() * 100)
			entry.LastCostTime = time.Now().Unix()
			return nil
		}
	}

	return nil
}

func (s *Server) processWantLists(ctx context.Context, config *pb.Config) error {
	defer s.CtxLog(ctx, "Complete processing")
	for _, list := range config.Lists {
		s.updateCosts(ctx, list)

		if list.GetType() != pb.WantList_ALL_IN {
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
				err := s.wantBridge.want(ctx, toUpdateToWanted.Want, list.GetRetireTime(), list.GetBudget())
				s.CtxLog(ctx, fmt.Sprintf("Updating %v to WANTED with error %v", toUpdateToWanted.Want, err))
				if err == nil {
					toUpdateToWanted.Status = pb.WantListEntry_WANTED
				}
			}

			if toUpdateToWanted == nil {
				s.CtxLog(ctx, fmt.Sprintf("Updating full wants for %v", list.GetName()))
				for _, v := range list.Wants {
					s.updateWant(ctx, v, list)
				}
			}

			list.LastProcessTime = time.Now().Unix()
		} else {
			active := true
			for _, w := range list.GetWants() {
				if w.Status == pb.WantListEntry_LIMBO {
					active = false
				}
			}
			s.CtxLog(ctx, fmt.Sprintf("Procesing %v with %v", list.GetName(), active))

			if active {
				for _, w := range list.GetWants() {
					if w.Status == pb.WantListEntry_UNPROCESSED {
						w.Status = pb.WantListEntry_WANTED
						err := s.updateWant(ctx, w, list)
						if err != nil {
							return err
						}
					}
				}
			} else {
				for _, w := range list.GetWants() {
					if w.Status == pb.WantListEntry_WANTED {
						w.Status = pb.WantListEntry_UNPROCESSED
						err := s.updateWant(ctx, w, list)
						if err != nil {
							return err
						}
					}
				}
			}
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
