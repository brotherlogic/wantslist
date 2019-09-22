package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/brotherlogic/goserver/utils"
	"google.golang.org/grpc"

	pbgd "github.com/brotherlogic/godiscogs"
	pbrc "github.com/brotherlogic/recordcollection/proto"
	pb "github.com/brotherlogic/wantslist/proto"

	//Needed to pull in gzip encoding init
	_ "google.golang.org/grpc/encoding/gzip"
)

func getRecordRep(ctx context.Context, id int32) string {
	host, port, err := utils.Resolve("recordcollection", "wantslist-cli")
	if err != nil {
		return fmt.Sprintf("Unable to reach collection: %v", err)
	}
	conn, err := grpc.Dial(host+":"+strconv.Itoa(int(port)), grpc.WithInsecure())
	defer conn.Close()

	if err != nil {
		return fmt.Sprintf("Unable to dial: %v", err)
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
	host, port, err := utils.Resolve("wantslist", "wantslist-cli")
	if err != nil {
		log.Fatalf("Unable to reach organiser: %v", err)
	}
	conn, err := grpc.Dial(host+":"+strconv.Itoa(int(port)), grpc.WithInsecure())
	defer conn.Close()

	if err != nil {
		log.Fatalf("Unable to dial: %v", err)
	}

	client := pb.NewWantServiceClient(conn)
	ctx, cancel := utils.BuildContext("wantslist-cli", "wantslist")
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
	case "file":
		list := &pb.WantList{Wants: []*pb.WantListEntry{}}

		file, err := os.Open(os.Args[2])
		if err != nil {
			log.Fatalf("Error reading file: %v", err)
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		scanner.Scan()
		list.Name = scanner.Text()

		index := int32(1)
		for scanner.Scan() {
			iv, err := strconv.Atoi(scanner.Text())
			if err != nil {
				log.Fatalf("Parse error: %v", err)
			}
			list.Wants = append(list.Wants, &pb.WantListEntry{Index: index, Want: int32(iv)})
			index++
		}

		_, err = client.AddWantList(ctx, &pb.AddWantListRequest{Add: list})
		if err != nil {
			log.Fatalf("Error adding wantlist: %v", err)
		}

	case "get":
		lists, err := client.GetWantList(ctx, &pb.GetWantListRequest{})
		if err != nil {
			log.Fatalf("Error getting wantlists: %v", err)
		}

		for i, list := range lists.Lists {
			fmt.Printf("List %v. %v (%v)\n", (i + 1), list.Name, time.Unix(list.LastProcessTime, 0))
			for _, entry := range list.Wants {
				fmt.Printf("  %v. %v (%v) [%v]\n", entry.Index, getRecordRep(ctx, entry.Want), entry.Want, entry.Status)
			}
		}
	}
}
