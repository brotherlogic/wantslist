package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"strconv"
	"time"

	"github.com/brotherlogic/goserver"
	"github.com/brotherlogic/goserver/utils"
	"github.com/brotherlogic/keystore/client"
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	pbgd "github.com/brotherlogic/godiscogs"
	pbg "github.com/brotherlogic/goserver/proto"
	pbrc "github.com/brotherlogic/recordcollection/proto"
	pbrw "github.com/brotherlogic/recordwants/proto"
	pb "github.com/brotherlogic/wantslist/proto"
)

const (
	// KEY - where the wantslists are stored
	KEY = "/github.com/brotherlogic/wantslist/config"
)

type rcBridge interface {
	getRecord(ctx context.Context, id int32) (*pbrc.Record, error)
}

type wantBridge interface {
	want(ctx context.Context, id int32) error
}

type prodRcBridge struct {
	dial func(server string) (*grpc.ClientConn, error)
}

func (p *prodRcBridge) getRecord(ctx context.Context, id int32) (*pbrc.Record, error) {
	conn, err := p.dial("recordcollection")
	defer conn.Close()

	if err != nil {
		return nil, err
	}

	client := pbrc.NewRecordCollectionServiceClient(conn)
	recs, err := client.GetRecords(ctx, &pbrc.GetRecordsRequest{Filter: &pbrc.Record{Release: &pbgd.Release{Id: id}}})

	if err != nil {
		return nil, err
	}

	if len(recs.GetRecords()) == 0 {
		return nil, fmt.Errorf("No records with %v id found", id)
	}

	return recs.GetRecords()[0], nil
}

type prodWantBridge struct{}

func (p *prodWantBridge) want(ctx context.Context, id int32) error {
	host, port, err := utils.Resolve("recordwants")
	if err != nil {
		return err
	}
	conn, err := grpc.Dial(host+":"+strconv.Itoa(int(port)), grpc.WithInsecure())
	defer conn.Close()

	if err != nil {
		return err
	}

	client := pbrw.NewWantServiceClient(conn)
	_, err = client.AddWant(ctx, &pbrw.AddWantRequest{ReleaseId: id, Superwant: true})
	return err
}

//Server main server type
type Server struct {
	*goserver.GoServer
	config     *pb.Config
	wantBridge wantBridge
	rcBridge   rcBridge
	listWait   time.Duration
}

// Init builds the server
func Init() *Server {
	s := &Server{
		&goserver.GoServer{},
		&pb.Config{},
		&prodWantBridge{},
		&prodRcBridge{},
		0,
	}
	// 168 hours is one week
	d, err := time.ParseDuration("1h")
	if err != nil {
		log.Fatalf("Error parsing duration: %v", err)
	}
	s.listWait = d
	s.rcBridge = &prodRcBridge{dial: s.DialMaster}
	return s
}

// DoRegister does RPC registration
func (s *Server) DoRegister(server *grpc.Server) {
	pb.RegisterWantServiceServer(server, s)
}

// ReportHealth alerts if we're not healthy
func (s *Server) ReportHealth() bool {
	return true
}

func (s *Server) save(ctx context.Context) {
	go s.KSclient.Save(ctx, KEY, s.config)
}

func (s *Server) load(ctx context.Context) error {
	config := &pb.Config{}
	data, _, err := s.KSclient.Read(ctx, KEY, config)

	if err != nil {
		return err
	}

	s.config = data.(*pb.Config)
	return nil
}

// Shutdown the server
func (s *Server) Shutdown(ctx context.Context) error {
	s.save(ctx)
	return nil
}

// Mote promotes/demotes this server
func (s *Server) Mote(ctx context.Context, master bool) error {
	if master {
		err := s.load(ctx)
		return err
	}

	return nil
}

// GetState gets the state of the server
func (s *Server) GetState() []*pbg.State {
	unprocCount := int64(0)
	wantedCount := int64(0)
	unknownCount := int64(0)
	total := int64(0)
	lowestProcessTime := time.Now().Unix()

	for _, list := range s.config.Lists {

		if list.LastProcessTime < lowestProcessTime {
			lowestProcessTime = list.LastProcessTime
		}

		total += int64(len(list.Wants))
		for _, entry := range list.Wants {
			switch entry.Status {
			case pb.WantListEntry_UNPROCESSED:
				unprocCount++
			case pb.WantListEntry_WANTED:
				wantedCount++
			default:
				unknownCount++
			}
		}
	}
	return []*pbg.State{
		&pbg.State{Key: "lists", Value: int64(len(s.config.Lists))},
		&pbg.State{Key: "unproc", Value: unprocCount},
		&pbg.State{Key: "wanted", Value: wantedCount},
		&pbg.State{Key: "unknown", Value: unknownCount},
		&pbg.State{Key: "total", Value: total},
		&pbg.State{Key: "last_processed", TimeValue: lowestProcessTime},
	}
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
	server.PrepServer()
	server.Register = server

	server.GoServer.KSclient = *keystoreclient.GetClient(server.DialMaster)

	server.RegisterServer("wantslist", false)

	server.RegisterRepeatingTask(server.prodProcess, "process_want_lists", time.Minute)

	fmt.Printf("%v", server.Serve())
}
