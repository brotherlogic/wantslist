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
	rcpb "github.com/brotherlogic/recordcollection/proto"
	pbrw "github.com/brotherlogic/recordwants/proto"
	pb "github.com/brotherlogic/wantslist/proto"
)

const (
	// KEY - where the wantslists are stored
	KEY = "/github.com/brotherlogic/wantslist/config"
)

type rcBridge interface {
	getRecord(ctx context.Context, id int32) (*rcpb.Record, error)
	getSpRecord(ctx context.Context, id int32) (*rcpb.Record, error)
}

type wantBridge interface {
	want(ctx context.Context, id int32, retire int64, budget string) error
	unwant(ctx context.Context, id int32, budget string) error
	get(ctx context.Context, id int32) (*pbrw.MasterWant, error)
}

type prodRcBridge struct {
	dial func(ctx context.Context, server string) (*grpc.ClientConn, error)
}

func (p *prodRcBridge) getRecord(ctx context.Context, id int32) (*rcpb.Record, error) {
	conn, err := p.dial(ctx, "recordcollection")
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	client := rcpb.NewRecordCollectionServiceClient(conn)
	ids, err := client.QueryRecords(ctx, &rcpb.QueryRecordsRequest{Query: &rcpb.QueryRecordsRequest_ReleaseId{int32(id)}})
	if err != nil {
		return nil, err
	}

	if len(ids.GetInstanceIds()) > 0 {
		rec, err := client.GetRecord(ctx, &rcpb.GetRecordRequest{InstanceId: ids.GetInstanceIds()[0]})
		if err != nil {
			return nil, err
		}
		return rec.GetRecord(), err
	}

	return nil, fmt.Errorf("Cannot locate %v", id)
}

func (p *prodRcBridge) getSpRecord(ctx context.Context, iid int32) (*rcpb.Record, error) {
	conn, err := p.dial(ctx, "recordcollection")
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	client := rcpb.NewRecordCollectionServiceClient(conn)
	ids, err := client.GetRecord(ctx, &rcpb.GetRecordRequest{InstanceId: iid})
	if err != nil {
		return nil, err
	}

	return ids.GetRecord(), nil
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

//Server main server type
type Server struct {
	*goserver.GoServer
	wantBridge wantBridge
	rcBridge   rcBridge
	lastRun    time.Time
}

// Init builds the server
func Init() *Server {
	s := &Server{
		GoServer:   &goserver.GoServer{},
		wantBridge: &prodWantBridge{},
		rcBridge:   &prodRcBridge{},
		lastRun:    time.Now(),
	}
	s.rcBridge = &prodRcBridge{dial: s.FDialServer}
	s.wantBridge = &prodWantBridge{dial: s.FDialServer}
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
	s.CtxLog(ctx, fmt.Sprintf("SAVING: %v", config))
	return s.KSclient.Save(ctx, KEY, config)
}

func (s *Server) load(ctx context.Context) (*pb.Config, error) {
	config := &pb.Config{}
	data, _, err := s.KSclient.Read(ctx, KEY, config)

	if err != nil {
		return nil, err
	}

	config = data.(*pb.Config)

	for _, list := range config.GetLists() {
		if list.GetName() == "digital" {
			list.Type = pb.WantList_ALL_IN
			list.Budget = "digital"
		}
		if list.GetName() == "digital_quick" {
			list.Type = pb.WantList_ALL_IN
			list.Budget = "digital_keep"
		}
		if list.GetName() == "The Fall" {
			list.Type = pb.WantList_RAPID
			for _, elem := range list.GetWants() {
				if elem.GetWant() == 143755 {
					elem.Want = 1437557
				}
			}
		}
	}

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

	server.Serve()
}
