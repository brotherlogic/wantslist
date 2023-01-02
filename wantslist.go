package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"time"

	"github.com/brotherlogic/goserver"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pbgd "github.com/brotherlogic/godiscogs"
	pbg "github.com/brotherlogic/goserver/proto"
	"github.com/brotherlogic/goserver/utils"
	rbc "github.com/brotherlogic/recordbudget/client"
	rcc "github.com/brotherlogic/recordcollection/client"
	rcpb "github.com/brotherlogic/recordcollection/proto"
	pbrw "github.com/brotherlogic/recordwants/proto"
	pb "github.com/brotherlogic/wantslist/proto"
)

const (
	// KEY - where the wantslists are stored
	KEY = "/github.com/brotherlogic/wantslist/config"
)

type wantBridge interface {
	want(ctx context.Context, id int32, retire int64, budget string) error
	unwant(ctx context.Context, id int32, budget string) error
	get(ctx context.Context, id int32) (*pbrw.MasterWant, error)
}

type prodRcBridge struct {
	dial func(ctx context.Context, server string) (*grpc.ClientConn, error)
}

type prodWantBridge struct {
	dial func(ctx context.Context, server string) (*grpc.ClientConn, error)
}

func (p *prodWantBridge) want(ctx context.Context, id int32, retire int64, budget string) error {
	conn, err := p.dial(ctx, "recordwants")
	if err != nil {
		return err
	}
	defer conn.Close()

	client := pbrw.NewWantServiceClient(conn)
	_, err = client.AddWant(ctx, &pbrw.AddWantRequest{ReleaseId: id, Budget: budget})
	if err != nil && status.Convert(err).Code() != codes.FailedPrecondition {
		return err
	}
	_, err = client.Update(ctx, &pbrw.UpdateRequest{Want: &pbgd.Release{Id: id}, Level: pbrw.MasterWant_LIST, RetireTime: retire, Budget: budget})
	return err
}

func (p *prodWantBridge) unwant(ctx context.Context, id int32, budget string) error {
	conn, err := p.dial(ctx, "recordwants")
	if err != nil {
		return err
	}
	defer conn.Close()

	client := pbrw.NewWantServiceClient(conn)
	_, err = client.AddWant(ctx, &pbrw.AddWantRequest{ReleaseId: id})
	if err != nil && status.Convert(err).Code() != codes.FailedPrecondition {
		return err
	}
	_, err = client.Update(ctx, &pbrw.UpdateRequest{Want: &pbgd.Release{Id: id}, Budget: budget, Level: pbrw.MasterWant_NEVER})
	return err
}

func (p *prodWantBridge) get(ctx context.Context, id int32) (*pbrw.MasterWant, error) {
	conn, err := p.dial(ctx, "recordwants")
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	client := pbrw.NewWantServiceClient(conn)
	want, err := client.GetWants(ctx, &pbrw.GetWantsRequest{ReleaseId: []int32{id}})
	if err != nil {
		return nil, err
	}

	if len(want.GetWant()) == 0 {
		return nil, fmt.Errorf("Cannot find wants for %v", id)
	}

	return want.GetWant()[0], err
}

// Server main server type
type Server struct {
	*goserver.GoServer
	wantBridge   wantBridge
	lastRun      time.Time
	rcclient     *rcc.RecordCollectionClient
	budgetClient *rbc.RecordBudgetClient
}

// Init builds the server
func Init() *Server {
	s := &Server{
		GoServer:   &goserver.GoServer{},
		wantBridge: &prodWantBridge{},
		lastRun:    time.Now().Add(-time.Hour * 2),
	}
	s.wantBridge = &prodWantBridge{dial: s.FDialServer}
	s.rcclient = &rcc.RecordCollectionClient{Gs: s.GoServer}
	s.budgetClient = &rbc.RecordBudgetClient{Gs: s.GoServer}
	return s
}

// DoRegister does RPC registration
func (s *Server) DoRegister(server *grpc.Server) {
	pb.RegisterWantServiceServer(server, s)
	rcpb.RegisterClientUpdateServiceServer(server, s)
}

// ReportHealth alerts if we're not healthy
func (s *Server) ReportHealth() bool {
	return true
}

func (s *Server) save(ctx context.Context, config *pb.Config) error {
	recordMetrics(config)
	return s.KSclient.Save(ctx, KEY, config)
}

func (s *Server) load(ctx context.Context) (*pb.Config, error) {
	config := &pb.Config{}
	data, _, err := s.KSclient.Read(ctx, KEY, config)

	if err != nil {
		return nil, err
	}

	config = data.(*pb.Config)

	recordMetrics(config)

	return config, nil
}

// Shutdown the server
func (s *Server) Shutdown(ctx context.Context) error {
	return nil
}

// Mote promotes/demotes this server
func (s *Server) Mote(ctx context.Context, master bool) error {
	return nil
}

// GetState gets the state of the server
func (s *Server) GetState() []*pbg.State {
	return []*pbg.State{}
}

func main() {
	var quiet = flag.Bool("quiet", false, "Show all output")
	flag.Parse()

	//Turn off logging
	if *quiet {
		log.SetFlags(0)
		log.SetOutput(ioutil.Discard)
	}
	server := Init()
	server.PrepServer("wantslist")
	server.Register = server

	err := server.RegisterServerV2(false)
	if err != nil {
		return
	}

	ctx, cancel := utils.ManualContext("wantslist-init", time.Minute)
	config, err := server.load(ctx)
	if err != nil {
		return
	}
	recordMetrics(config)
	cancel()

	server.Serve()
}
