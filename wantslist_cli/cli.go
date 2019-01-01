package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/brotherlogic/goserver/utils"
	"google.golang.org/grpc"

	pbgd "github.com/brotherlogic/godiscogs"
	pbgs "github.com/brotherlogic/goserver/proto"
	pbrc "github.com/brotherlogic/recordcollection/proto"
	pbt "github.com/brotherlogic/tracer/proto"
	pb "github.com/brotherlogic/wantslist/proto"

	//Needed to pull in gzip encoding init
	_ "google.golang.org/grpc/encoding/gzip"
)

func getRecordRep(ctx context.Context, id int32) string {
	host, port, err := utils.Resolve("recordcollection")
	if err != nil {
		log.Fatalf("Unable to reach collection: %v", err)
	}
	conn, err := grpc.Dial(host+":"+strconv.Itoa(int(port)), grpc.WithInsecure())
	defer conn.Close()

	if err != nil {
		log.Fatalf("Unable to dial: %v", err)
	}

	client := pbrc.NewRecordCollectionServiceClient(conn)
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	rel, err := client.GetRecords(ctx, &pbrc.GetRecordsRequest{Filter: &pbrc.Record{Release: &pbgd.Release{Id: id}}})
	if err != nil || len(rel.GetRecords()) == 0 {
		return ""
	}
	return fmt.Sprintf("%v", rel.GetRecords()[0].GetRelease().Title)
}

func main() {
	host, port, err := utils.Resolve("wantslist")
	if err != nil {
		log.Fatalf("Unable to reach organiser: %v", err)
	}
	conn, err := grpc.Dial(host+":"+strconv.Itoa(int(port)), grpc.WithInsecure())
	defer conn.Close()

	if err != nil {
		log.Fatalf("Unable to dial: %v", err)
	}

	client := pb.NewWantServiceClient(conn)
	ctx, cancel := utils.BuildContext("wantslist-cli", "wantslist", pbgs.ContextType_LONG)
	defer cancel()

	switch os.Args[1] {
	case "add":
		list := &pb.WantList{Name: os.Args[2], Wants: []*pb.WantListEntry{}}
		for i, v := range os.Args[3:] {
			val, err := strconv.Atoi(v)
			if err != nil {
				log.Fatalf("Cannot parse %v", v)
			}
			list.Wants = append(list.Wants, &pb.WantListEntry{Index: int32(i + 1), Want: int32(val)})
		}

		_, err := client.AddWantList(ctx, &pb.AddWantListRequest{Add: list})
		if err != nil {
			log.Fatalf("Error adding wantlist: %v", err)
		}
	case "get":
		lists, err := client.GetWantList(ctx, &pb.GetWantListRequest{})
		if err != nil {
			log.Fatalf("Error getting wantlists: %v", err)
		}

		for i, list := range lists.Lists {
			fmt.Printf("List %v. %v\n", (i + 1), list.Name)
			for _, entry := range list.Wants {
				fmt.Printf("  %v. %v (%v)\n", entry.Index, getRecordRep(ctx, entry.Want), entry.Want)
			}
		}
	}

	utils.SendTrace(ctx, "End of CLI", time.Now(), pbt.Milestone_END, "recordwants-cli")
}
