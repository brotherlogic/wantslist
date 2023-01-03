package main

import (
	"fmt"
	"log"
	"math"
	"sort"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	rbpb "github.com/brotherlogic/recordbudget/proto"
	pbrc "github.com/brotherlogic/recordcollection/proto"
	pbrw "github.com/brotherlogic/recordwants/proto"
	pb "github.com/brotherlogic/wantslist/proto"
)

var (
	togoMetric = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "wantslist_togo",
		Help: "The number of outstanding wants",
	}, []string{"list", "budget"})
	costMetric = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "wantslist_cost",
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
	activeMetric = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "wantslist_active",
		Help: "The number of outstanding wants",
	}, []string{"list", "budget"})
)

func recordMetrics(config *pb.Config) {
	for _, list := range config.GetLists() {
		togo := 0
		remaining := 0
		active := 0
		for _, entry := range list.GetWants() {
			if entry.Status != pb.WantListEntry_WANTED && entry.Status == pb.WantListEntry_COMPLETE {
				togo++
				remaining += int(entry.GetEstimatedCost())
			}
			if entry.Status == pb.WantListEntry_WANTED {
				active++
			}
		}
		if active > 0 {
			activeMetric.With(prometheus.Labels{"list": list.GetName(), "budget": list.GetBudget()}).Set(float64(active))
		}
		togoMetric.With(prometheus.Labels{"list": list.GetName(), "budget": list.GetBudget()}).Set(float64(togo))
		costMetric.With(prometheus.Labels{"list": list.GetName(), "budget": list.GetBudget()}).Set(float64(remaining))
	}
}

func (s *Server) prodProcess(ctx context.Context, config *pb.Config) error {
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

	return nil, status.Errorf(codes.NotFound, "Cannot locate %v", id)
}
func (s *Server) updateWantOld(ctx context.Context, v *pb.WantListEntry, list *pb.WantList) error {
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

		//
		budget, err := s.budgetClient.GetBudget(ctx, &rbpb.GetBudgetRequest{Budget: list.GetBudget()})
		if err != nil {
			return err
		}
		if budget.GetChosenBudget().GetRemaining() <= 0 {
			s.CtxLog(ctx, fmt.Sprintf("Unwanting %v becuase budget %v has no money in it", list.GetName(), list.GetBudget()))
			for _, w := range list.GetWants() {
				if w.Status == pb.WantListEntry_WANTED {
					w.Status = pb.WantListEntry_UNPROCESSED
				}
			}

			continue
		}

		sort.SliceStable(list.Wants, func(i2, j2 int) bool {
			return list.Wants[i2].Index < list.Wants[j2].Index
		})
		hasLimbo := false
		for _, entry := range list.GetWants() {
			if entry.Status == pb.WantListEntry_LIMBO {
				hasLimbo = true
				break
			}
		}

		if hasLimbo {
			for _, w := range list.GetWants() {
				if w.Status == pb.WantListEntry_WANTED {
					w.Status = pb.WantListEntry_UNPROCESSED
				}
			}
			continue
		}

		isValidate := time.Since(time.Unix(list.GetLastValidate(), 0)) > time.Hour*24

		if isValidate {
			switch list.GetType() {
			case pb.WantList_ALL_IN:
				for _, w := range list.GetWants() {
					want, err := s.wantBridge.get(ctx, w.GetWant())
					if err != nil && status.Code(err) != codes.NotFound {
						return err
					}

					if want != nil && want.GetCurrentState() == pbrw.MasterWant_WANTED && w.GetStatus() != pb.WantListEntry_WANTED {
						s.wantBridge.unwant(ctx, w.GetWant(), list.GetBudget())
					} else if (want == nil || want.GetCurrentState() != pbrw.MasterWant_WANTED) && w.GetStatus() == pb.WantListEntry_WANTED {
						s.wantBridge.want(ctx, w.GetWant(), list.GetRetireTime(), list.GetBudget())
					}

					_, err = s.getRecord(ctx, w.GetWant())
					if err == nil || status.Code(err) != codes.NotFound {
						w.Status = pb.WantListEntry_COMPLETE
					}

					if w.Status == pb.WantListEntry_UNPROCESSED {
						w.Status = pb.WantListEntry_WANTED
					}
				}
			case pb.WantList_STANDARD, pb.WantList_RAPID:
				prior := pb.WantListEntry_COMPLETE
				for _, entry := range list.GetWants() {
					if entry.GetStatus() == pb.WantListEntry_UNPROCESSED && prior == pb.WantListEntry_COMPLETE {
						entry.Status = pb.WantListEntry_WANTED
						break
					}
				}
			case pb.WantList_YEARLY:
				days := 365 / len(list.GetWants())
				for i, entry := range list.GetWants() {
					want, err := s.wantBridge.get(ctx, entry.GetWant())
					if err != nil && status.Code(err) != codes.NotFound {
						return err
					}
					if want != nil && want.GetCurrentState() == pbrw.MasterWant_WANTED && entry.GetStatus() != pb.WantListEntry_WANTED {
						s.wantBridge.unwant(ctx, entry.GetWant(), list.GetBudget())
					} else if (want == nil || want.GetCurrentState() != pbrw.MasterWant_WANTED) && entry.GetStatus() == pb.WantListEntry_WANTED {
						s.wantBridge.want(ctx, entry.GetWant(), list.GetRetireTime(), list.GetBudget())
					}

					r, err := s.getRecord(ctx, entry.GetWant())
					s.CtxLog(ctx, fmt.Sprintf("GOT record: %v -> %v (%v)", entry, err, r))

					if err == nil {
						entry.Status = pb.WantListEntry_COMPLETE
					} else if status.Code(err) == codes.NotFound && entry.GetStatus() == pb.WantListEntry_COMPLETE {
						entry.Status = pb.WantListEntry_UNPROCESSED
					}

					if time.Now().YearDay() > days*i {
						if entry.Status == pb.WantListEntry_UNPROCESSED {
							entry.Status = pb.WantListEntry_WANTED
						}
					}
					log.Printf("Now %v", entry)
				}
			}

			list.LastValidate = time.Now().Unix()
		}
	}

	config = s.cleanWantlists(ctx, config)
	return s.save(ctx, config)
}

func (s *Server) cleanWantlists(ctx context.Context, config *pb.Config) *pb.Config {
	newwantlists := []*pb.WantList{}
	for _, list := range config.Lists {
		if int32(time.Now().Year()) == list.GetYear() {
			newwantlists = append(newwantlists, list)
		}
	}

	config.Lists = newwantlists
	return config
}
