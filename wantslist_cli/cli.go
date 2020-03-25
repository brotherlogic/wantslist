package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/brotherlogic/goserver/utils"
	"google.golang.org/grpc"

	pb "github.com/brotherlogic/wantslist/proto"

	//Needed to pull in gzip encoding init
	_ "google.golang.org/grpc/encoding/gzip"
	"google.golang.org/grpc/resolver"
)

func init() {
	resolver.Register(&utils.DiscoveryClientResolverBuilder{})
}

func main() {
	conn, err := grpc.Dial("discovery:///wantslist", grpc.WithInsecure(), grpc.WithBalancerName("my_pick_first"))
	defer conn.Close()

	if err != nil {
		log.Fatalf("Unable to dial: %v", err)
	}

	client := pb.NewWantServiceClient(conn)
	ctx, cancel := utils.BuildContext("wantslist-cli", "wantslist")
	defer cancel()

	switch os.Args[1] {
	case "get":
		lists, err := client.GetWantList(ctx, &pb.GetWantListRequest{})
		if err != nil {
			log.Fatalf("Error getting wantlists: %v", err)
		}

		for i, list := range lists.Lists {
			fmt.Printf("List %v. %v\n", (i + 1), list.Name)
			for _, entry := range list.Wants {
				fmt.Printf("  %v. %v (%v)\n", entry.Index, entry.Status, entry.Want)
			}
		}

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

	}
}
